package batcher

import (
	"errors"
	"time"

	"github.com/rafael/tshark-streaming-ingestion/services/worker-postgres-go/internal/shape"
)

// ErrBufferFull is returned by Add when the buffer has reached its cap.
// The caller must activate backpressure — the rejected item is not buffered.
var ErrBufferFull = errors.New("buffer full")

// Batcher accumulates rows and flushes them in batches, either when the batch
// reaches batchSize or when flushInterval elapses since the last flush.
//
// Not goroutine-safe: paho.mqtt.golang delivers messages to the default
// publish handler sequentially from a single goroutine, matching the Python
// worker's single-threaded model. If a background flusher is added later,
// a mutex must be introduced.
type Batcher struct {
	buffer        []shape.Row
	batchSize     int
	flushInterval time.Duration
	bufferCap     int
	lastFlush     time.Time
	flushLocked   bool
	onFlush       func([]shape.Row)
}

func New(batchSize int, flushInterval time.Duration, bufferCap int, onFlush func([]shape.Row)) *Batcher {
	return &Batcher{
		buffer:        make([]shape.Row, 0, batchSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		bufferCap:     bufferCap,
		lastFlush:     time.Now(),
		onFlush:       onFlush,
	}
}

func (b *Batcher) Add(row shape.Row) error {
	if len(b.buffer) >= b.bufferCap {
		return ErrBufferFull
	}
	b.buffer = append(b.buffer, row)
	if len(b.buffer) >= b.batchSize || time.Since(b.lastFlush) >= b.flushInterval {
		b.flush()
	}
	return nil
}

// flush is reentrant-safe: if onFlush calls Add (directly or transitively),
// the nested Add sees flushLocked and does not recurse into another flush.
func (b *Batcher) flush() {
	if len(b.buffer) == 0 || b.flushLocked {
		return
	}
	b.flushLocked = true
	batch := b.buffer
	b.buffer = make([]shape.Row, 0, b.batchSize)
	b.lastFlush = time.Now()
	b.onFlush(batch)
	b.flushLocked = false
}

func (b *Batcher) BufferSize() int {
	return len(b.buffer)
}
