import json
import paho.mqtt.client as mqtt
import psycopg
import time
from tshark_shared.logging import logger
from tshark_shared.config import worker_settings
from tshark_shared.mqtt import TOPIC_PACKETS, TOPIC_DEADLETTER
from tshark_shared.mem import start_memory_metrics
from worker_postgres.shape import shape_ek_to_row
from worker_postgres.db import insert_batch
from worker_postgres.batcher import Batcher, BufferFull
from worker_postgres.metrics import start_metrics_server, record_msg_consumed, record_rows_inserted, record_batch_flush, record_dead_letter, record_db_error, set_buffer_size


class BufferFull(Exception):
    pass


class BufferFull(Exception):
    pass


class WorkerConsumer:
    def __init__(self):
        self.client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=worker_settings.MQTT_CLIENT_ID)
        self.client.enable_clean_session = not worker_settings.MQTT_SESSION_PERSISTENT
        self.client.enable_logger()
        self.client.on_connect = self._on_connect
        self.client.on_message = self._on_message

        self.conn = psycopg.connect(worker_settings.POSTGRES_DSN)

        self.batcher = Batcher(
            batch_size=worker_settings.DB_BATCH_SIZE,
            flush_interval=worker_settings.DB_BATCH_FLUSH_SECONDS,
            on_flush=self._flush_to_db,
            buffer_cap=worker_settings.DB_BUFFER_CAP,
        )
        self.backpressure_active = False

        start_metrics_server(worker_settings.METRICS_PORT)
        start_memory_metrics("worker-postgres")
        logger.info(f"Metrics server started on port {worker_settings.METRICS_PORT}")

    def _flush_to_db(self, rows):
        max_retries = 3
        for attempt in range(max_retries):
            try:
                insert_batch(self.conn, rows)
                logger.info(f"Flushed {len(rows)} rows to DB")
                record_rows_inserted(len(rows))
                record_batch_flush()
                self.backpressure_active = False  # release backpressure on success
                return
            except Exception as e:
                logger.error(f"DB flush failed (attempt {attempt + 1}/{max_retries}): {e}")
                record_db_error()
                if attempt < max_retries - 1:
                    time.sleep(2 ** attempt)  # exponential backoff: 1s, 2s, 4s
                else:
                    self.backpressure_active = True  # activate backpressure on failure
                    raise

    def _on_connect(self, client, userdata, flags, reason_code, properties):
        client.subscribe(TOPIC_PACKETS, qos=worker_settings.MQTT_QOS)
        logger.info(f"Connected to MQTT broker, subscribed to {TOPIC_PACKETS}")

    def _on_message(self, client, userdata, msg):
        if self.backpressure_active:
            logger.warning("Backpressure active, skipping message")
            return

        try:
            row = shape_ek_to_row(msg.payload.decode())
            self.batcher.add(row)
            record_msg_consumed()
            set_buffer_size(len(self.batcher.buffer))
        except BufferFull:
            self.backpressure_active = True
            logger.error(f"Buffer cap {worker_settings.DB_BUFFER_CAP} reached, backpressure activated")
            set_buffer_size(len(self.batcher.buffer))
        except Exception as e:
            logger.warning(f"Failed to parse ek line: {e}")
            record_dead_letter()
            dead_letter = json.dumps({"raw_ek_line": msg.payload.decode(), "error": str(e), "ts": msg.timestamp})
            client.publish(TOPIC_DEADLETTER, dead_letter, qos=worker_settings.MQTT_QOS)

    def start(self):
        self.client.connect(worker_settings.MQTT_HOST, worker_settings.MQTT_PORT)
        logger.info("Starting MQTT consumer...")
        self.client.loop_forever()


if __name__ == "__main__":
    worker = WorkerConsumer()
    worker.start()