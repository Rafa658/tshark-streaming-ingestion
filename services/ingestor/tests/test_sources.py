import pytest
from ingestor.sources import LiveTsharkSource, PcapReplaySource


def test_live_source_interface():
    source = LiveTsharkSource("en0")
    assert source.iface == "en0"
    assert source.bpf_filter == ""


def test_live_source_with_filter():
    source = LiveTsharkSource("en0", "tcp port 443")
    assert source.iface == "en0"
    assert source.bpf_filter == "tcp port 443"


def test_pcap_source_interface():
    source = PcapReplaySource("/data/pcap/sample.pcap")
    assert source.pcap_path == "/data/pcap/sample.pcap"
    assert not source.loop


def test_pcap_source_with_loop():
    source = PcapReplaySource("/data/pcap/sample.pcap", loop=True)
    assert source.pcap_path == "/data/pcap/sample.pcap"
    assert source.loop

def test_pcap_source_default_rate_is_unthrottled():
    source = PcapReplaySource("/data/pcap/sample.pcap")
    assert source.rate == 0.0


def test_pcap_source_with_rate():
    source = PcapReplaySource("/data/pcap/sample.pcap", loop=True, rate=100.0)
    assert source.rate == 100.0
    assert source.loop


def test_throttle_passthrough_when_rate_is_zero():
    from ingestor.sources import _throttle

    out = list(_throttle(iter("abcde"), 0))
    assert out == list("abcde")


def test_throttle_limits_emission_rate():
    import time as _time

    from ingestor.sources import _throttle

    start = _time.monotonic()
    out = list(_throttle(iter(range(10)), rate=200.0))
    elapsed = _time.monotonic() - start

    assert out == list(range(10))
    # 10 items at 200/s must take at least ~45ms (allow scheduler slack)
    assert elapsed >= 0.035, f"throttle did not delay: {elapsed:.4f}s"


def test_throttle_does_not_burst_after_slow_consumer():
    """A stalled consumer must not cause a catch-up burst."""
    import time as _time

    from ingestor.sources import _throttle

    gen = _throttle(iter(range(3)), rate=1000.0)
    next(gen)
    _time.sleep(0.05)  # consumer stalls well past the schedule

    start = _time.monotonic()
    next(gen)
    next(gen)
    elapsed = _time.monotonic() - start
    # after resync the next item still waits its full interval, no free burst
    assert elapsed >= 0.0005


# ---------------------------------------------------------------------------
# F4 regression: PcapReplaySource must spawn tshark exactly once and loop the
# cached ek lines, not re-spawn tshark per loop pass.
# ---------------------------------------------------------------------------


def _fake_ek_lines(lines):
    """Return a fake _read_ek_lines replacement that counts calls."""
    calls = []

    def fake(cmd):
        calls.append(cmd)
        yield from lines

    return fake, calls


def test_pcap_replay_spawns_tshark_once_even_when_looping(monkeypatch):
    """F4: tshark must be spawned exactly once, not once per loop pass."""
    fake, calls = _fake_ek_lines(["line1", "line2", "line3"])
    monkeypatch.setattr("ingestor.sources._read_ek_lines", fake)

    source = PcapReplaySource("/data/pcap/sample.pcap", loop=True, rate=10000.0)
    gen = source.lines()
    for _ in range(10):
        next(gen)

    assert len(calls) == 1, f"tshark spawned {len(calls)} times, expected 1"


def test_pcap_replay_no_loop_plays_cache_once(monkeypatch):
    """Without loop, the cache is played exactly once and tshark spawns once."""
    fake, calls = _fake_ek_lines(["a", "b", "c"])
    monkeypatch.setattr("ingestor.sources._read_ek_lines", fake)

    source = PcapReplaySource("/data/pcap/sample.pcap", loop=False, rate=0)
    lines = list(source.lines())

    assert lines == ["a", "b", "c"]
    assert len(calls) == 1


def test_pcap_replay_empty_cache_yields_nothing(monkeypatch):
    """An empty pcap must produce no lines, not infinite-loop re-spawning tshark."""
    import signal

    fake, calls = _fake_ek_lines([])
    monkeypatch.setattr("ingestor.sources._read_ek_lines", fake)

    source = PcapReplaySource("/data/pcap/sample.pcap", loop=True, rate=100.0)

    def _alarm(signum, frame):
        raise TimeoutError("empty cache with loop=True hangs")

    old = signal.signal(signal.SIGALRM, _alarm)
    signal.alarm(2)
    try:
        lines = list(source.lines())
    finally:
        signal.alarm(0)
        signal.signal(signal.SIGALRM, old)

    assert lines == []
    assert len(calls) == 1

