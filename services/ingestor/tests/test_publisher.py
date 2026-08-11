import pytest
from unittest.mock import MagicMock, patch
from ingestor.publisher import MQTTPublisher


def test_publisher_initialization():
    publisher = MQTTPublisher()
    assert publisher.client is not None


def test_publisher_publish_line():
    publisher = MQTTPublisher()
    publisher.client = MagicMock()
    publisher.client.publish.return_value = None

    publisher.publish_line('{"test": "data"}')
    publisher.client.publish.assert_called_once()


@patch('ingestor.publisher.ingestor_settings')
def test_publisher_connect(mock_settings):
    mock_settings.MQTT_HOST = "localhost"
    mock_settings.MQTT_PORT = 1883

    publisher = MQTTPublisher()
    publisher.client = MagicMock()

    publisher.start()
    publisher.client.connect.assert_called_once_with("localhost", 1883)