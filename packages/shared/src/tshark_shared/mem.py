"""Process memory metrics collector."""

import os
import threading

import psutil
from prometheus_client import Gauge

from tshark_shared.config import settings

__all__ = ["start_memory_metrics", "stop_memory_metrics", "mem_gauge"]

mem_gauge = Gauge(
    "process_memory_bytes",
    "Process RSS memory in bytes",
    ["service"],
)

_process = psutil.Process(os.getpid())
# A single reusable event: the previous implementation allocated a fresh
# threading.Event() on every iteration purely to sleep, and offered no way to
# stop the collector.
_stop = threading.Event()
_thread: threading.Thread | None = None


def _collect(service: str, interval: float) -> None:
    while not _stop.is_set():
        mem_gauge.labels(service).set(_process.memory_info().rss)
        _stop.wait(interval)


def start_memory_metrics(service: str, interval: float | None = None) -> None:
    """Start sampling RSS into the process_memory_bytes gauge."""
    global _thread
    if _thread is not None and _thread.is_alive():
        return
    _stop.clear()
    _thread = threading.Thread(
        target=_collect,
        args=(service, interval or settings.MEM_METRICS_INTERVAL_SECONDS),
        daemon=True,
        name="memory-metrics",
    )
    _thread.start()


def stop_memory_metrics(timeout: float = 2.0) -> None:
    """Stop the collector. Primarily used by tests and graceful shutdown."""
    global _thread
    _stop.set()
    if _thread is not None:
        _thread.join(timeout=timeout)
        _thread = None
