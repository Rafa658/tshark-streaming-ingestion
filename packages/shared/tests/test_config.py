"""Tests for shared settings loading.

Regression coverage for two defects: Settings had no SettingsConfigDict so a
.env file was never read, and the MQTT_TOPIC_* env vars were shadowed by
hardcoded constants in mqtt.py and therefore silently ignored.
"""

import importlib

from tshark_shared.config import IngestorSettings, Settings, WorkerPostgresSettings


def test_defaults_are_stable():
    s = Settings()
    assert s.MQTT_HOST == "mosquitto"
    assert s.MQTT_PORT == 1883
    assert s.MQTT_QOS == 1
    assert s.LOG_LEVEL == "INFO"
    assert s.METRICS_PORT == 8000


def test_env_var_overrides_default(monkeypatch):
    monkeypatch.setenv("MQTT_HOST", "broker.example.test")
    monkeypatch.setenv("METRICS_PORT", "9111")
    s = Settings()
    assert s.MQTT_HOST == "broker.example.test"
    assert s.METRICS_PORT == 9111


def test_env_file_is_loaded(tmp_path, monkeypatch):
    env_file = tmp_path / ".env"
    env_file.write_text("MQTT_HOST=from-env-file\nLOG_LEVEL=WARNING\n", encoding="utf-8")
    monkeypatch.chdir(tmp_path)

    s = Settings()
    assert s.MQTT_HOST == "from-env-file"
    assert s.LOG_LEVEL == "WARNING"


def test_unknown_env_keys_are_ignored(tmp_path, monkeypatch):
    # A single .env holds the union of every service's config, so a key that is
    # not part of a given Settings class must not raise.
    env_file = tmp_path / ".env"
    env_file.write_text("MQTT_HOST=ok\nSOME_UNRELATED_KEY=value\n", encoding="utf-8")
    monkeypatch.chdir(tmp_path)

    assert Settings().MQTT_HOST == "ok"


def test_service_settings_have_distinct_client_ids():
    assert IngestorSettings().MQTT_CLIENT_ID == "ingestor-1"
    assert WorkerPostgresSettings().MQTT_CLIENT_ID == "worker-postgres-1"


def test_topics_follow_settings(monkeypatch):
    monkeypatch.setenv("MQTT_TOPIC_PACKETS", "custom/packets/v9")
    monkeypatch.setenv("MQTT_TOPIC_DEADLETTER", "custom/dead/v9")

    import tshark_shared.config as config_module
    import tshark_shared.mqtt as mqtt_module

    importlib.reload(config_module)
    importlib.reload(mqtt_module)
    try:
        assert mqtt_module.TOPIC_PACKETS == "custom/packets/v9"
        assert mqtt_module.TOPIC_DEADLETTER == "custom/dead/v9"
    finally:
        monkeypatch.undo()
        importlib.reload(config_module)
        importlib.reload(mqtt_module)


def test_topic_defaults_match_prd():
    import tshark_shared.mqtt as mqtt_module

    assert mqtt_module.TOPIC_PACKETS == "tshark/packets/v1"
    assert mqtt_module.TOPIC_DEADLETTER == "tshark/dead-letter/v1"
