import pytest
from worker_postgres.shape import shape_ek_to_row
from datetime import datetime
from ipaddress import IPv4Address


def test_shape_ek_to_row_basic():
    ek_line = '{"@timestamp": "2026-08-09T12:34:56.789Z", "layers": {"frame": {"frame.protocols": "eth:ip:tcp:http", "frame.len": 1234}, "ip": {"ip.src": "192.168.1.100", "ip.dst": "10.0.0.200", "ip.proto": "6"}, "tcp": {"tcp.srcport": "443", "tcp.dstport": "54321"}}}'
    row = shape_ek_to_row(ek_line)
    assert row.ts.year == 2026
    assert row.src_ip == IPv4Address("192.168.1.100")
    assert row.dst_ip == IPv4Address("10.0.0.200")
    assert row.src_port == 443
    assert row.dst_port == 54321
    assert row.proto == "eth:ip:tcp:http"
    assert row.length == 1234
    assert "layers" in row.payload


def test_shape_ek_missing_required_field():
    ek_line = '{"@timestamp": "2026-08-09T12:34:56.789Z", "layers": {"frame": {"frame.protocols": "eth:ip", "frame.len": 100}}}'
    with pytest.raises(ValueError):
        shape_ek_to_row(ek_line)


def test_shape_ek_invalid_ip():
    ek_line = '{"@timestamp": "2026-08-09T12:34:56.789Z", "layers": {"frame": {"frame.protocols": "eth:ip", "frame.len": 100}, "ip": {"ip.src": "invalid", "ip.dst": "10.0.0.1"}}}'
    with pytest.raises(ValueError):
        shape_ek_to_row(ek_line)