from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    MQTT_HOST: str = "mosquitto"
    MQTT_PORT: int = 1883
    MQTT_TOPIC_PACKETS: str = "tshark/packets/v1"
    MQTT_TOPIC_DEADLETTER: str = "tshark/dead-letter/v1"
    MQTT_QOS: int = 1
    MQTT_CLIENT_ID: str = "default-client"
    MQTT_SESSION_PERSISTENT: bool = True

    LOG_LEVEL: str = "INFO"
    METRICS_PORT: int = 8000


class IngestorSettings(Settings):
    MQTT_CLIENT_ID: str = "ingestor-1"
    TSHARK_INTERFACE: str = "en0"
    TSHARK_BPF_FILTER: str = ""
    TSHARK_PCAP_PATH: str = ""
    TSHARK_REPLAY_LOOP: bool = False


class WorkerPostgresSettings(Settings):
    MQTT_CLIENT_ID: str = "worker-postgres-1"
    POSTGRES_DSN: str = "postgresql://tshark_user:tshark_password@postgres:5432/tshark_db"
    DB_BATCH_SIZE: int = 500
    DB_BATCH_FLUSH_SECONDS: int = 1
    DB_BUFFER_CAP: int = 50000


settings = Settings()
ingestor_settings = IngestorSettings()
worker_settings = WorkerPostgresSettings()