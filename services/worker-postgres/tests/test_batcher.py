import time

import pytest
from worker_postgres.batcher import Batcher, BufferFull


def test_batcher_size_trigger():
    flushes = []
    batcher = Batcher(batch_size=3, flush_interval=10, on_flush=flushes.append, buffer_cap=100)
    batcher.add("a")
    batcher.add("b")
    assert len(flushes) == 0
    batcher.add("c")
    assert flushes == [["a", "b", "c"]]


def test_batcher_time_trigger():
    flushes = []
    batcher = Batcher(batch_size=100, flush_interval=1, on_flush=flushes.append, buffer_cap=100)
    batcher.add("a")
    assert len(flushes) == 0
    time.sleep(1.1)
    batcher.add("b")
    assert len(flushes) == 1
    assert "a" in flushes[0]


def test_batcher_buffer_cap_raises_buffer_full():
    """Buffer cap is the backpressure trigger, not a soft guard."""
    flushes = []
    batcher = Batcher(batch_size=100, flush_interval=10, on_flush=flushes.append, buffer_cap=2)
    batcher.add("a")
    batcher.add("b")
    with pytest.raises(BufferFull):
        batcher.add("c")
    assert len(flushes) == 0
    assert batcher.buffer_size == 2  # rejected item must not be buffered


def test_batcher_flush_is_reentrant_safe():
    """A flush that re-enters add() must not recurse into a second flush."""
    seen = []

    def on_flush(batch):
        seen.append(batch)
        batcher.add("reentrant")

    batcher = Batcher(batch_size=2, flush_interval=10, on_flush=on_flush, buffer_cap=100)
    batcher.add("a")
    batcher.add("b")
    assert seen == [["a", "b"]]
    assert batcher.buffer_size == 1


def test_batcher_empty_flush_is_noop():
    flushes = []
    batcher = Batcher(batch_size=10, flush_interval=10, on_flush=flushes.append, buffer_cap=10)
    batcher.flush()
    assert flushes == []
