"""Canonical MQTT topic names.

Per the PRD these are the single source of truth for topic names, but they are
derived from settings rather than hardcoded so the documented
MQTT_TOPIC_PACKETS / MQTT_TOPIC_DEADLETTER environment variables actually take
effect. Previously the constants here shadowed the settings and the env vars
were silently ignored.
"""

from tshark_shared.config import settings

TOPIC_PACKETS: str = settings.MQTT_TOPIC_PACKETS
TOPIC_DEADLETTER: str = settings.MQTT_TOPIC_DEADLETTER
