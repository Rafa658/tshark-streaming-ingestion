"""Tests for the process memory metrics collector."""

import threading
import time

from tshark_shared.mem import mem_gauge, start_memory_metrics, stop_memory_metrics


def _collector_threads():
    return [t for t in threading.enumerate() if t.name == "memory-metrics" and t.is_alive()]


def test_gauge_is_populated():
    try:
        start_memory_metrics("unit-test", interval=0.01)
        time.sleep(0.1)
        assert mem_gauge.labels("unit-test")._value.get() > 0
    finally:
        stop_memory_metrics()


def test_collector_can_be_stopped():
    start_memory_metrics("unit-test", interval=0.01)
    assert _collector_threads()

    stop_memory_metrics()
    assert not _collector_threads()


def test_start_is_idempotent():
    try:
        start_memory_metrics("unit-test", interval=0.01)
        start_memory_metrics("unit-test", interval=0.01)
        assert len(_collector_threads()) == 1
    finally:
        stop_memory_metrics()


def test_collector_thread_is_daemon():
    try:
        start_memory_metrics("unit-test", interval=0.01)
        assert all(t.daemon for t in _collector_threads())
    finally:
        stop_memory_metrics()
