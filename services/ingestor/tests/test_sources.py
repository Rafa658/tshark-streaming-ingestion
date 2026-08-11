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