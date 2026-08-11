import json

import pytest
from ipaddress import IPv4Address, IPv6Address
from worker_postgres.shape import shape_ek_to_row


def _ek(**overrides) -> str:
    """Build a line in the real `tshark -T ek` shape (underscored keys, ms epoch)."""
    doc = {
        "timestamp": "1770000000789",
        "layers": {
            "frame": {
                "frame_frame_protocols": "eth:ethertype:ip:tcp",
                "frame_frame_len": "1234",
            },
            "ip": {"ip_ip_src": "192.168.1.100", "ip_ip_dst": "10.0.0.200", "ip_ip_proto": "6"},
            "tcp": {"tcp_tcp_srcport": "443", "tcp_tcp_dstport": "54321"},
        },
    }
    doc.update(overrides)
    return json.dumps(doc)


def test_shape_ek_to_row_basic():
    row = shape_ek_to_row(_ek())
    assert row.ts.year == 2026
    assert row.ts.tzinfo is not None
    assert row.src_ip == IPv4Address("192.168.1.100")
    assert row.dst_ip == IPv4Address("10.0.0.200")
    assert row.src_port == 443
    assert row.dst_port == 54321
    assert row.proto == "eth:ethertype:ip:tcp"
    assert row.length == 1234
    assert "layers" in row.payload


def test_shape_ek_timestamp_is_utc_milliseconds():
    row = shape_ek_to_row(_ek(timestamp="0"))
    assert row.ts.timestamp() == 0
    assert str(row.ts.tzinfo) == "UTC"


def test_shape_ek_udp_ports():
    layers = {
        "frame": {"frame_frame_protocols": "eth:ip:udp", "frame_frame_len": "60"},
        "ip": {"ip_ip_src": "192.168.1.1", "ip_ip_dst": "8.8.8.8"},
        "udp": {"udp_udp_srcport": "53", "udp_udp_dstport": "5353"},
    }
    row = shape_ek_to_row(_ek(layers=layers))
    assert row.src_port == 53
    assert row.dst_port == 5353


def test_shape_ek_ipv6():
    layers = {
        "frame": {"frame_frame_protocols": "eth:ipv6:tcp", "frame_frame_len": "80"},
        "ipv6": {"ipv6_ipv6_src": "2001:db8::1", "ipv6_ipv6_dst": "2001:db8::2"},
        "tcp": {"tcp_tcp_srcport": "80", "tcp_tcp_dstport": "8080"},
    }
    row = shape_ek_to_row(_ek(layers=layers))
    assert row.src_ip == IPv6Address("2001:db8::1")
    assert row.dst_ip == IPv6Address("2001:db8::2")


def test_shape_ek_no_transport_layer_has_null_ports():
    layers = {
        "frame": {"frame_frame_protocols": "eth:ip:icmp", "frame_frame_len": "98"},
        "ip": {"ip_ip_src": "192.168.1.1", "ip_ip_dst": "192.168.1.2"},
    }
    row = shape_ek_to_row(_ek(layers=layers))
    assert row.src_port is None
    assert row.dst_port is None


def test_shape_ek_missing_required_field():
    layers = {"frame": {"frame_frame_protocols": "eth:ip", "frame_frame_len": "100"}}
    with pytest.raises(ValueError):
        shape_ek_to_row(_ek(layers=layers))


def test_shape_ek_invalid_ip():
    layers = {
        "frame": {"frame_frame_protocols": "eth:ip", "frame_frame_len": "100"},
        "ip": {"ip_ip_src": "invalid", "ip_ip_dst": "10.0.0.1"},
    }
    with pytest.raises(ValueError):
        shape_ek_to_row(_ek(layers=layers))


def test_shape_ek_missing_timestamp_raises():
    doc = json.loads(_ek())
    del doc["timestamp"]
    with pytest.raises(ValueError):
        shape_ek_to_row(json.dumps(doc))
