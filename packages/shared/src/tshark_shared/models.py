from dataclasses import dataclass
from datetime import datetime
from ipaddress import IPv4Address, IPv6Address
from typing import Union

IPAddress = Union[IPv4Address, IPv6Address]


@dataclass
class PacketRow:
    ts: datetime
    src_ip: IPAddress
    dst_ip: IPAddress
    src_port: int | None
    dst_port: int | None
    proto: str
    length: int
    payload: dict