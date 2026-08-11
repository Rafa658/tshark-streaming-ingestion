import abc
import subprocess
from typing import Iterator


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

        proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, text=True)
        for line in proc.stdout:
            line = line.strip()
            if not line:
                continue
            # ek format outputs paired lines: index line + data line
            # skip index lines (they start with {"index")
            if line.startswith('{"index"'):
                continue
            yield line


class PcapReplaySource(Source):
    def __init__(self, pcap_path: str, loop: bool = False):
        self.pcap_path = pcap_path
        self.loop = loop

    def lines(self) -> Iterator[str]:
        while True:
            cmd = ["tshark", "-r", self.pcap_path, "-T", "ek"]
            proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, text=True)
            for line in proc.stdout:
                line = line.strip()
                if not line:
                    continue
                # ek format outputs paired lines: index line + data line
                # skip index lines (they start with {"index")
                if line.startswith('{"index"'):
                    continue
                yield line
            if not self.loop:
                break