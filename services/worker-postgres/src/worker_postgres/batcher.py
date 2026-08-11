import time
from collections import deque
from typing import Callable


class BufferFull(Exception):
    pass


class Batcher:
    def __init__(self, batch_size: int, flush_interval: int, on_flush: Callable[[list], None], buffer_cap: int):
        self.batch_size = batch_size
        self.flush_interval = flush_interval
        self.on_flush = on_flush
        self.buffer_cap = buffer_cap
        self.buffer = deque()
        self.last_flush = time.time()
        self.flush_locked = False

    def add(self, item):
        if len(self.buffer) >= self.buffer_cap:
            raise BufferFull(f"Buffer cap {self.buffer_cap} reached, backpressure needed")
        self.buffer.append(item)
        if len(self.buffer) >= self.batch_size or time.time() - self.last_flush >= self.flush_interval:
            self.flush()

    def flush(self):
        if not self.buffer or self.flush_locked:
            return
        self.flush_locked = True
        try:
            batch = list(self.buffer)
            self.buffer.clear()
            self.last_flush = time.time()
            self.on_flush(batch)
        finally:
            self.flush_locked = False

    @property
    def buffer_size(self) -> int:
        return len(self.buffer)