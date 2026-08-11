import json
from datetime import datetime, timezone
from ipaddress import IPv4Address, IPv6Address, AddressValueError
from tshark_shared.models import PacketRow, IPAddress


def shape_ek_to_row(ek_line: str) -> PacketRow:
    ek = json.loads(ek_line)
    layers = ek.get("layers", {})

    # ek format uses "timestamp" (milliseconds since epoch), not "@timestamp"
    ts_ms = ek.get("timestamp", "")
    ts = datetime.fromtimestamp(int(ts_ms) / 1000, tz=timezone.utc)

    frame = layers.get("frame", {})
    ip_layer = layers.get("ip") or layers.get("ipv6", {})
    tcp_layer = layers.get("tcp")
    udp_layer = layers.get("udp")

    # ek format uses underscores: "ip_ip_src" instead of "ip.src"
    src_ip_str = ip_layer.get("ip_ip_src") or ip_layer.get("ipv6_ipv6_src")
    dst_ip_str = ip_layer.get("ip_ip_dst") or ip_layer.get("ipv6_ipv6_dst")

    if not src_ip_str or not dst_ip_str:
        raise ValueError(f"Missing IP addresses: src={src_ip_str}, dst={dst_ip_str}")

    src_ip: IPAddress
    dst_ip: IPAddress
    try:
        src_ip = IPv4Address(src_ip_str) if ":" not in src_ip_str else IPv6Address(src_ip_str)
        dst_ip = IPv4Address(dst_ip_str) if ":" not in dst_ip_str else IPv6Address(dst_ip_str)
    except (AddressValueError, ValueError):
        raise ValueError(f"Invalid IP: src={src_ip_str}, dst={dst_ip_str}")

    src_port = tcp_layer.get("tcp_tcp_srcport") if tcp_layer else (udp_layer.get("udp_udp_srcport") if udp_layer else None)
    dst_port = tcp_layer.get("tcp_tcp_dstport") if tcp_layer else (udp_layer.get("udp_udp_dstport") if udp_layer else None)

    proto = frame.get("frame_frame_protocols", "")
    length = frame.get("frame_frame_len", 0)

    return PacketRow(
        ts=ts,
        src_ip=src_ip,
        dst_ip=dst_ip,
        src_port=int(src_port) if src_port else None,
        dst_port=int(dst_port) if dst_port else None,
        proto=proto,
        length=int(length),
        payload=ek,
    )