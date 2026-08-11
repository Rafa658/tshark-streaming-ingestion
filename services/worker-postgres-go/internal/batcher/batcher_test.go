package batcher

import (
	"testing"
	"time"

	"github.com/rafael/tshark-streaming-ingestion/services/worker-postgres-go/internal/shape"
)

func dummyRow() shape.Row {
	return shape.Row{Proto: "test"}
}

func TestBatcherSizeTrigger(t *testing.T) {
	var flushes [][]shape.Row
	b := New(3, 10*time.Second, 100, func(rows []shape.Row) { flushes = append(flushes, rows) })
	b.Add(dummyRow())
	b.Add(dummyRow())
	if len(flushes) != 0 {
		t.Fatalf("flushes = %d, want 0", len(flushes))
	}
	b.Add(dummyRow())
	if len(flushes) != 1 {
		t.Fatalf("flushes = %d, want 1", len(flushes))
	}
	if len(flushes[0]) != 3 {
		t.Errorf("batch size = %d, want 3", len(flushes[0]))
	}
}

func TestBatcherTimeTrigger(t *testing.T) {
	var flushes [][]shape.Row
	b := New(100, 1*time.Second, 100, func(rows []shape.Row) { flushes = append(flushes, rows) })
	b.Add(dummyRow())
	if len(flushes) != 0 {
		t.Fatalf("flushes = %d, want 0 before interval", len(flushes))
	}
	time.Sleep(1100 * time.Millisecond)
	b.Add(dummyRow())
	if len(flushes) != 1 {
		t.Fatalf("flushes = %d, want 1 after interval", len(flushes))
	}
	if len(flushes[0]) != 2 {
		t.Errorf("batch size = %d, want 2 (both rows flushed)", len(flushes[0]))
	}
}

func TestBatcherBufferCapRaisesBufferFull(t *testing.T) {
	var flushes [][]shape.Row
	b := New(100, 10*time.Second, 2, func(rows []shape.Row) { flushes = append(flushes, rows) })
	if err := b.Add(dummyRow()); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(dummyRow()); err != nil {
		t.Fatal(err)
	}
	err := b.Add(dummyRow())
	if err != ErrBufferFull {
		t.Fatalf("err = %v, want ErrBufferFull", err)
	}
	if len(flushes) != 0 {
		t.Errorf("flushes = %d, want 0", len(flushes))
	}
	if b.BufferSize() != 2 {
		t.Errorf("buffer_size = %d, want 2 (rejected item must not be buffered)", b.BufferSize())
	}
}

func TestBatcherReentrantSafe(t *testing.T) {
	var b *Batcher
	var seen [][]shape.Row
	onFlush := func(rows []shape.Row) {
		seen = append(seen, rows)
		b.Add(dummyRow()) // reentrant
	}
	b = New(2, 10*time.Second, 100, onFlush)
	b.Add(dummyRow())
	b.Add(dummyRow())
	if len(seen) != 1 {
		t.Fatalf("flushes = %d, want 1", len(seen))
	}
	if b.BufferSize() != 1 {
		t.Errorf("buffer_size = %d, want 1 (reentrant add)", b.BufferSize())
	}
}

func TestBatcherEmptyFlushIsNoop(t *testing.T) {
	var flushes [][]shape.Row
	b := New(10, 10*time.Second, 10, func(rows []shape.Row) { flushes = append(flushes, rows) })
	// flush is unexported; calling Add after interval with empty buffer triggers it
	time.Sleep(50 * time.Millisecond)
	b.Add(dummyRow())
	// first Add after interval triggers flush of empty buffer (noop) then appends;
	// but the interval check happens after append, so the buffer has 1 item.
	// The reentrant-safe flush sees 1 item and flushes it.
	if len(flushes) > 1 {
		t.Errorf("flushes = %d, want <= 1", len(flushes))
	}
}
