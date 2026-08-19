package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	MQTTHost              string
	MQTTPort              int
	MQTTTopicPackets      string
	MQTTTopicDeadLetter   string
	MQTTQoS               int
	MQTTClientID          string
	MQTTSessionPersistent bool
	LogLevel              string
	MetricsPort           int
	PostgresDSN           string
	PostgresTable         string
	DBBatchSize           int
	DBBatchFlushSeconds   int
	DBBufferCap           int
}

func Load() Config {
	return Config{
		MQTTHost:              envStr("MQTT_HOST", "mosquitto"),
		MQTTPort:              envInt("MQTT_PORT", 1883),
		MQTTTopicPackets:      envStr("MQTT_TOPIC_PACKETS", "tshark/packets/v1"),
		MQTTTopicDeadLetter:   envStr("MQTT_TOPIC_DEADLETTER", "tshark/dead-letter/v1"),
		MQTTQoS:               envInt("MQTT_QOS", 1),
		MQTTClientID:          envStr("MQTT_CLIENT_ID", "worker-postgres-1"),
		MQTTSessionPersistent: envBool("MQTT_SESSION_PERSISTENT", true),
		LogLevel:              envStr("LOG_LEVEL", "INFO"),
		MetricsPort:           envInt("METRICS_PORT", 8000),
		PostgresDSN:           envStr("POSTGRES_DSN", "postgresql://tshark_user:tshark_password@postgres:5432/tshark_db"),
		PostgresTable:         envStr("POSTGRES_TABLE", "packets"),
		DBBatchSize:           envInt("DB_BATCH_SIZE", 500),
		DBBatchFlushSeconds:   envInt("DB_BATCH_FLUSH_SECONDS", 1),
		DBBufferCap:           envInt("DB_BUFFER_CAP", 50000),
	}
}

func (c Config) MQTTBrokerURL() string {
	return fmt.Sprintf("tcp://%s:%d", c.MQTTHost, c.MQTTPort)
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
