from unittest.mock import MagicMock, patch

from ingestor.publisher import MQTTPublisher, PENDING_SWEEP_THRESHOLD


def _info(published: bool):
    i = MagicMock()
    i.is_published.return_value = published
    return i


def test_publisher_initialization():
    publisher = MQTTPublisher()
    assert publisher.client is not None
    assert len(publisher.pending) == 0


def test_publisher_publish_line():
    publisher = MQTTPublisher()
    publisher.client = MagicMock()
    publisher.client.publish.return_value = _info(False)

    publisher.publish_line('{"test": "data"}')
    publisher.client.publish.assert_called_once()
    assert len(publisher.pending) == 1


def test_pending_drains_delivered_messages():
    """Regression: pending must not grow unbounded during loop replay."""
    publisher = MQTTPublisher()
    publisher.client = MagicMock()
    publisher.client.publish.side_effect = lambda *a, **k: _info(True)

    for _ in range(1000):
        publisher.publish_line("x")

    # every prior info reports delivered, so only the newest remains un-drained
    assert len(publisher.pending) == 1


def test_pending_retains_undelivered_messages():
    publisher = MQTTPublisher()
    publisher.client = MagicMock()
    publisher.client.publish.side_effect = lambda *a, **k: _info(False)

    for _ in range(50):
        publisher.publish_line("x")

    assert len(publisher.pending) == 50


def test_pending_sweep_handles_out_of_order_completion():
    """Head blocked but tail delivered must still be reclaimed via the sweep."""
    publisher = MQTTPublisher()
    publisher.client = MagicMock()

    blocked_head = _info(False)
    publisher.pending.append(blocked_head)
    for _ in range(PENDING_SWEEP_THRESHOLD + 10):
        publisher.pending.append(_info(True))

    publisher.client.publish.return_value = _info(False)
    publisher.publish_line("x")

    # head survives (still unpublished), delivered tail is swept, plus the new info
    assert len(publisher.pending) == 2
    assert publisher.pending[0] is blocked_head


def test_wait_pending_clears_buffer():
    publisher = MQTTPublisher()
    publisher.client = MagicMock()
    for _ in range(5):
        publisher.pending.append(_info(False))

    publisher.wait_pending()
    assert len(publisher.pending) == 0


@patch("ingestor.publisher.ingestor_settings")
def test_publisher_connect(mock_settings):
    mock_settings.MQTT_HOST = "localhost"
    mock_settings.MQTT_PORT = 1883
    mock_settings.LOG_LEVEL = "INFO"

    publisher = MQTTPublisher()
    publisher.client = MagicMock()

    publisher.start()
    publisher.client.connect.assert_called_once_with("localhost", 1883)
