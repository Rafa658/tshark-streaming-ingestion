"""Regression tests for the backpressure path.

`consumer.py` previously redefined `BufferFull` locally, shadowing the class that
`Batcher.add()` actually raises. `except BufferFull` could therefore never match, so
the buffer-cap branch was unreachable and backpressure never engaged. These tests pin
the corrected behaviour.
"""

import json
from unittest.mock import MagicMock

import pytest
from worker_postgres.batcher import Batcher, BufferFull
from worker_postgres import consumer as consumer_mod


def test_consumer_bufferfull_is_the_batcher_bufferfull():
    """The identity that the original shadowing bug broke."""
    assert consumer_mod.BufferFull is BufferFull


def test_bufferfull_raised_by_batcher_is_caught_by_consumer_handler():
    batcher = Batcher(batch_size=100, flush_interval=10, on_flush=lambda b: None, buffer_cap=1)
    batcher.add("a")

    caught = False
    try:
        batcher.add("b")
    except consumer_mod.BufferFull:
        caught = True
    assert caught, "consumer's BufferFull must catch what Batcher raises"


class _FakeConsumer:
    """Exercises the real _on_message logic without MQTT or Postgres."""

    def __init__(self, cap):
        self.backpressure_active = False
        self.batcher = Batcher(
            batch_size=10_000, flush_interval=10_000, on_flush=lambda b: None, buffer_cap=cap
        )
        self.dead_letters = []

    _on_message = consumer_mod.WorkerConsumer._on_message


def _ek_line(ts="1770000000000"):
    return json.dumps(
        {
            "timestamp": ts,
            "layers": {
                "frame": {"frame_frame_protocols": "eth:ip:tcp", "frame_frame_len": "100"},
                "ip": {"ip_ip_src": "10.0.0.1", "ip_ip_dst": "10.0.0.2"},
                "tcp": {"tcp_tcp_srcport": "1", "tcp_tcp_dstport": "2"},
            },
        }
    )


def _msg(payload: str):
    m = MagicMock()
    m.payload = payload.encode()
    m.timestamp = 0
    return m


def test_backpressure_activates_when_buffer_cap_reached():
    c = _FakeConsumer(cap=2)
    client = MagicMock()

    c._on_message(client, None, _msg(_ek_line()))
    c._on_message(client, None, _msg(_ek_line()))
    assert c.backpressure_active is False
    assert c.batcher.buffer_size == 2

    c._on_message(client, None, _msg(_ek_line()))
    assert c.backpressure_active is True, "buffer cap must engage backpressure"
    assert c.batcher.buffer_size == 2, "over-cap message must not be buffered"


def test_messages_are_dropped_while_backpressure_active():
    c = _FakeConsumer(cap=2)
    client = MagicMock()
    c.backpressure_active = True

    c._on_message(client, None, _msg(_ek_line()))
    assert c.batcher.buffer_size == 0
    client.publish.assert_not_called()


def test_unparseable_line_goes_to_dead_letter_not_backpressure():
    c = _FakeConsumer(cap=10)
    client = MagicMock()

    c._on_message(client, None, _msg("not json at all"))

    assert c.backpressure_active is False
    client.publish.assert_called_once()
    topic, payload = client.publish.call_args[0][0], client.publish.call_args[0][1]
    assert "dead-letter" in topic
    assert json.loads(payload)["raw_ek_line"] == "not json at all"
