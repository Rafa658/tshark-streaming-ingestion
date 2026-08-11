import json
import psycopg
from psycopg.types.json import Json
from tshark_shared.models import PacketRow


def insert_batch(conn: psycopg.Connection, rows: list[PacketRow]) -> None:
    query = """
        INSERT INTO packets (ts, src_ip, dst_ip, src_port, dst_port, proto, length, payload)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
    """
    values = [
        (
            r.ts,
            str(r.src_ip),
            str(r.dst_ip),
            r.src_port,
            r.dst_port,
            r.proto,
            r.length,
            Json(r.payload),
        )
        for r in rows
    ]
    with conn.cursor() as cur:
        cur.executemany(query, values)
        conn.commit()