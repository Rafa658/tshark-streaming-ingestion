from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Base settings shared by every service.

    Values are read from the process environment, falling back to a .env file
    at the repo root. Unknown keys are ignored so a single .env can hold the
    union of every service's configuration.
    """

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )

    MQTT_HOST: str = "mosquitto"
    MQTT_PORT: int = 1883
    MQTT_TOPIC_PACKETS: str = "tshark/packets/v1"
    MQTT_TOPIC_DEADLETTER: str = "tshark/dead-letter/v1"
    MQTT_QOS: int = 1
    MQTT_CLIENT_ID: str = "default-client"
    MQTT_SESSION_PERSISTENT: bool = True

    LOG_LEVEL: str = "INFO"
    METRICS_PORT: int = 8000
    MEM_METRICS_INTERVAL_SECONDS: float = 5.0


class IngestorSettings(Settings):
    MQTT_CLIENT_ID: str = "ingestor-1"
    TSHARK_INTERFACE: str = "en0"
    TSHARK_BPF_FILTER: str = ""
    TSHARK_PCAP_PATH: str = ""
    TSHARK_REPLAY_LOOP: bool = False


class WorkerPostgresSettings(Settings):
    MQTT_CLIENT_ID: str = "worker-postgres-1"
    # NOTE: dev-only default. Carrying credentials here is a known limitation
    # documented in the CHANGELOG; override POSTGRES_DSN for any real deployment.
    POSTGRES_DSN: str = "postgresql://tshark_user:tshark_password@postgres:5432/tshark_db"
    DB_BATCH_SIZE: int = 500
    DB_BATCH_FLUSH_SECONDS: float = 1.0
    DB_BUFFER_CAP: int = 50000


settings = Settings()
ingestor_settings = IngestorSettings()
worker_settings = WorkerPostgresSettings()
