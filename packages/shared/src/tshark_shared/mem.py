import os
import threading
import psutil
from prometheus_client import Gauge


mem_gauge = Gauge(
    "process_memory_bytes",
    "Process RSS memory in bytes",
    ["service"],
)

_process = psutil.Process(os.getpid())


def _update(service: str):
    while True:
        mem_gauge.labels(service).set(_process.memory_info().rss)
        threading.Event().wait(5)


def start_memory_metrics(service: str) -> None:
    t = threading.Thread(target=_update, args=(service,), daemon=True)
    t.start()