import re

import psycopg
from psycopg import sql
from psycopg.types.json import Json
from tshark_shared.config import worker_settings
from tshark_shared.models import PacketRow

_IDENT_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


def _table_ident(name: str) -> sql.Identifier:
    # table name comes from env, so validate before it reaches a composed statement
    if not _IDENT_RE.match(name):
        raise ValueError(f"Invalid table name: {name!r}")
    return sql.Identifier(name)


def insert_batch(
    conn: psycopg.Connection, rows: list[PacketRow], table: str | None = None
) -> None:
    ident = _table_ident(table or worker_settings.POSTGRES_TABLE)
    query = sql.SQL(
        """
        INSERT INTO {} (ts, src_ip, dst_ip, src_port, dst_port, proto, length, payload)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
        """
    ).format(ident)
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
