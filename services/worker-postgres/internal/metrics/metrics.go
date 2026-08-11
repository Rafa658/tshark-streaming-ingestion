package metrics

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const service = "worker-postgres"

var (
	msgsConsumed  prometheus.Counter
	rowsInserted  prometheus.Counter
	batchFlushes  prometheus.Counter
	deadLetters   prometheus.Counter
	dbErrors      prometheus.Counter
	bufferSize    prometheus.Gauge
	processMemory prometheus.Gauge
)

func init() {
	labels := prometheus.Labels{"service": service}

	msgsConsumed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "worker_msgs_consumed_total", Help: "Total MQTT messages consumed",
		ConstLabels: labels,
	})
	rowsInserted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "worker_rows_inserted_total", Help: "Total rows inserted into DB",
		ConstLabels: labels,
	})
	batchFlushes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "worker_batch_flush_total", Help: "Total batch flushes",
		ConstLabels: labels,
	})
	deadLetters = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "worker_deadletter_total", Help: "Total dead-letter messages",
		ConstLabels: labels,
	})
	dbErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "worker_db_errors_total", Help: "Total DB errors",
		ConstLabels: labels,
	})
	bufferSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "worker_buffer_size", Help: "Current buffer size",
		ConstLabels: labels,
	})
	processMemory = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "process_memory_bytes", Help: "Process RSS memory in bytes",
		ConstLabels: labels,
	})

	prometheus.MustRegister(
		msgsConsumed, rowsInserted, batchFlushes,
		deadLetters, dbErrors, bufferSize, processMemory,
	)
}

// StartServer serves /metrics and /healthz on the given port.
func StartServer(port int) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	go http.ListenAndServe(":"+strconv.Itoa(port), mux)
}

// StartMemoryUpdater polls RSS every 5 seconds and updates process_memory_bytes.
// Reads cgroup memory (matching docker stats) on Linux; falls back to
// /proc/self/statm RSS if cgroup is unavailable (e.g. macOS dev).
func StartMemoryUpdater() {
	go func() {
		for {
			if rss, ok := readMemoryBytes(); ok {
				processMemory.Set(float64(rss))
			}
			time.Sleep(5 * time.Second)
		}
	}()
}

func readMemoryBytes() (uint64, bool) {
	// cgroup v2 (Docker, Kubernetes)
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.current"); err == nil {
		if n, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			return n, true
		}
	}
	// cgroup v1 (older Docker)
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes"); err == nil {
		if n, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			return n, true
		}
	}
	// fallback: /proc/self/statm RSS (includes shared code pages)
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * uint64(os.Getpagesize()), true
}

func RecordMsgConsumed()       { msgsConsumed.Inc() }
func RecordRowsInserted(n int) { rowsInserted.Add(float64(n)) }
func RecordBatchFlush()        { batchFlushes.Inc() }
func RecordDeadLetter()        { deadLetters.Inc() }
func RecordDBError()           { dbErrors.Inc() }
func SetBufferSize(n int)      { bufferSize.Set(float64(n)) }
