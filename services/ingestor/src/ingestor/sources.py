import abc
import subprocess
import time
from typing import Iterator


def _throttle(lines: Iterator[str], rate: float) -> Iterator[str]:
    """Emit at most `rate` lines/sec using a monotonic schedule.

    Schedule-based rather than sleep-per-line so a slow consumer doesn't accumulate drift.
    """
    if rate <= 0:
        yield from lines
        return
    interval = 1.0 / rate
    next_due = time.monotonic()
    for line in lines:
        now = time.monotonic()
        if now < next_due:
            time.sleep(next_due - now)
            next_due += interval
        else:
            # fell behind - resync instead of bursting to catch up
            next_due = now + interval
        yield line


def _read_ek_lines(cmd: list[str]) -> Iterator[str]:
    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, text=True)
    try:
        for line in proc.stdout:
            line = line.strip()
            if not line:
                continue
            # ek emits paired lines: an index line then the data line; skip the index
            if line.startswith('{"index"'):
                continue
            yield line
    finally:
        if proc.stdout:
            proc.stdout.close()
        proc.wait()


class Source(abc.ABC):
    @abc.abstractmethod
    def lines(self) -> Iterator[str]:
        pass


class LiveTsharkSource(Source):
    def __init__(self, iface: str, bpf_filter: str = ""):
        self.iface = iface
        self.bpf_filter = bpf_filter

    def lines(self) -> Iterator[str]:
        cmd = ["tshark", "-i", self.iface, "-T", "ek", "-l"]
        if self.bpf_filter:
            cmd.extend(["-f", self.bpf_filter])
        yield from _read_ek_lines(cmd)


class PcapReplaySource(Source):
    def __init__(self, pcap_path: str, loop: bool = False, rate: float = 0.0):
        self.pcap_path = pcap_path
        self.loop = loop
        self.rate = rate

    def _load_ek_lines(self) -> list[str]:
        # ponytail: in-memory cache; spill to temp file if pcap ever exceeds ~100k lines
        cmd = ["tshark", "-r", self.pcap_path, "-T", "ek"]
        return list(_read_ek_lines(cmd))

    def lines(self) -> Iterator[str]:
        cached = self._load_ek_lines()
        if not cached:
            return
        while True:
            yield from _throttle(iter(cached), self.rate)
            if not self.loop:
                break
