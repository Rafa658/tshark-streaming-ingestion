import paho.mqtt.client as mqtt
from collections import deque
from tshark_shared.logging import logger
from tshark_shared.config import ingestor_settings
from tshark_shared.mqtt import TOPIC_PACKETS
from tshark_shared.mem import start_memory_metrics
from ingestor.metrics import start_metrics_server, record_line_read, record_msg_published

# above this many un-drained infos, fall back to a full sweep (out-of-order completions)
PENDING_SWEEP_THRESHOLD = 10_000


class MQTTPublisher:
    def __init__(self):
        self.client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=ingestor_settings.MQTT_CLIENT_ID)
        if ingestor_settings.LOG_LEVEL.upper() == "DEBUG":
            self.client.enable_logger()
        self.pending = deque()

    def _drain_pending(self):
        # amortized O(1): QoS1 acks arrive roughly in publish order, so drain from the head
        while self.pending and self.pending[0].is_published():
            self.pending.popleft()
        # head blocked but tail completed - sweep once rather than growing unbounded
        if len(self.pending) > PENDING_SWEEP_THRESHOLD:
            self.pending = deque(p for p in self.pending if not p.is_published())

    def publish_line(self, line: str):
        record_line_read()
        # drain before append: the fresh info is never published yet, so skip scanning it
        self._drain_pending()
        info = self.client.publish(TOPIC_PACKETS, line, qos=ingestor_settings.MQTT_QOS)
        self.pending.append(info)
        record_msg_published()

    def wait_pending(self):
        for info in self.pending:
            info.wait_for_publish(timeout=5)
        self.pending.clear()
        logger.info("All messages delivered")

    def start(self):
        start_metrics_server(ingestor_settings.METRICS_PORT)
        start_memory_metrics("ingestor")
        logger.info(f"Metrics server started on port {ingestor_settings.METRICS_PORT}")
        self.client.connect(ingestor_settings.MQTT_HOST, ingestor_settings.MQTT_PORT)
        logger.info("Ingestor publisher started...")
        self.client.loop_start()

    def stop(self):
        self.wait_pending()
        self.client.loop_stop()
        self.client.disconnect()