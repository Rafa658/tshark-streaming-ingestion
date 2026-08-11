package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rafael/tshark-streaming-ingestion/services/worker-postgres-go/internal/batcher"
	"github.com/rafael/tshark-streaming-ingestion/services/worker-postgres-go/internal/config"
	"github.com/rafael/tshark-streaming-ingestion/services/worker-postgres-go/internal/db"
	"github.com/rafael/tshark-streaming-ingestion/services/worker-postgres-go/internal/metrics"
	"github.com/rafael/tshark-streaming-ingestion/services/worker-postgres-go/internal/shape"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check DB connectivity and exit")
	flag.Parse()

	cfg := config.Load()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if *healthcheck {
		os.Exit(runHealthcheck(cfg))
	}
	if err := run(cfg); err != nil {
		slog.Error("worker failed", "error", err)
		os.Exit(1)
	}
}

func runHealthcheck(cfg config.Config) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return 1
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return 1
	}
	return 0
}

func run(cfg config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	metrics.StartServer(cfg.MetricsPort)
	metrics.StartMemoryUpdater()
	slog.Info("metrics server started", "port", cfg.MetricsPort)

	w := &worker{cfg: cfg, pool: pool}
	w.batcher = batcher.New(
		cfg.DBBatchSize,
		time.Duration(cfg.DBBatchFlushSeconds)*time.Second,
		cfg.DBBufferCap,
		w.flushToDB,
	)

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTTBrokerURL()).
		SetClientID(cfg.MQTTClientID).
		SetCleanSession(!cfg.MQTTSessionPersistent).
		SetAutoReconnect(true).
		SetOnConnectHandler(w.onConnect).
		SetDefaultPublishHandler(w.onMessage)

	if cfg.LogLevel == "DEBUG" {
		mqtt.DEBUG = slog.NewLogLogger(slog.Default().Handler(), slog.LevelDebug)
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("connect mqtt: %w", token.Error())
	}

	slog.Info("worker started",
		"mqtt_broker", cfg.MQTTBrokerURL(), "table", cfg.PostgresTable)
	select {}
}

type worker struct {
	cfg                config.Config
	pool               *pgxpool.Pool
	batcher            *batcher.Batcher
	backpressureActive bool
}

func (w *worker) onConnect(c mqtt.Client) {
	c.Subscribe(w.cfg.MQTTTopicPackets, byte(w.cfg.MQTTQoS), nil)
	slog.Info("subscribed", "topic", w.cfg.MQTTTopicPackets)
}

func (w *worker) onMessage(c mqtt.Client, msg mqtt.Message) {
	if w.backpressureActive {
		slog.Warn("backpressure active, skipping message")
		return
	}

	row, err := shape.Shape(msg.Payload())
	if err != nil {
		slog.Warn("failed to parse ek line", "error", err)
		metrics.RecordDeadLetter()
		deadLetter, _ := json.Marshal(map[string]any{
			"raw_ek_line": string(msg.Payload()),
			"error":       err.Error(),
			"ts":          time.Now().Unix(),
		})
		c.Publish(w.cfg.MQTTTopicDeadLetter, byte(w.cfg.MQTTQoS), false, deadLetter)
		return
	}

	if err := w.batcher.Add(row); err != nil {
		if err == batcher.ErrBufferFull {
			w.backpressureActive = true
			slog.Error("buffer cap reached, backpressure activated",
				"cap", w.cfg.DBBufferCap)
			metrics.SetBufferSize(w.batcher.BufferSize())
		}
		return
	}

	metrics.RecordMsgConsumed()
	metrics.SetBufferSize(w.batcher.BufferSize())
}

func (w *worker) flushToDB(rows []shape.Row) {
	const maxRetries = 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := db.InsertBatch(ctx, w.pool, rows, w.cfg.PostgresTable)
		cancel()
		if err == nil {
			slog.Info("flushed rows to DB", "count", len(rows))
			metrics.RecordRowsInserted(len(rows))
			metrics.RecordBatchFlush()
			w.backpressureActive = false
			return
		}
		slog.Error("DB flush failed",
			"attempt", attempt+1, "max", maxRetries, "error", err)
		metrics.RecordDBError()
		if attempt < maxRetries-1 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		} else {
			w.backpressureActive = true
		}
	}
}
