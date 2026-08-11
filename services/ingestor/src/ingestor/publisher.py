import paho.mqtt.client as mqtt
from tshark_shared.logging import logger
from tshark_shared.config import ingestor_settings
from tshark_shared.mqtt import TOPIC_PACKETS
from tshark_shared.mem import start_memory_metrics
from ingestor.metrics import start_metrics_server, record_line_read, record_msg_published


class MQTTPublisher:
    def __init__(self):
        self.client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=ingestor_settings.MQTT_CLIENT_ID)
        self.client.enable_logger()
        self.pending = []

    def publish_line(self, line: str):
        record_line_read()
        info = self.client.publish(TOPIC_PACKETS, line, qos=ingestor_settings.MQTT_QOS)
        # drop already-delivered infos; without this the list grows unbounded in loop mode
        self.pending = [p for p in self.pending if not p.is_published()]
        self.pending.append(info)
        record_msg_published()

    def wait_pending(self):
        for info in self.pending:
            info.wait_for_publish(timeout=5)
        self.pending.clear()
        logger.info(f"All messages delivered")

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