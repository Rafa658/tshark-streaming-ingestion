"""Tests for JSON logging setup.

Regression coverage: logging.py used to configure loguru at import time with a
hardcoded level of INFO and a human-readable format, so LOG_LEVEL was dead
config and output was not machine-parseable (PRD §16.6 requires JSON).
"""

import json

from tshark_shared.logging import configure_logging, logger


def test_output_is_json(capsys):
    configure_logging("unit-test")
    logger.info("structured message")

    line = capsys.readouterr().out.strip()
    payload = json.loads(line)  # must not raise
    assert payload["record"]["message"] == "structured message"


def test_service_name_is_bound(capsys):
    configure_logging("ingestor")
    logger.info("tagged")

    payload = json.loads(capsys.readouterr().out.strip())
    assert payload["record"]["extra"]["service"] == "ingestor"


def test_level_is_honored(capsys):
    configure_logging("unit-test", level="WARNING")
    logger.info("should be filtered out")
    logger.warning("should appear")

    lines = [line for line in capsys.readouterr().out.splitlines() if line.strip()]
    assert len(lines) == 1
    assert json.loads(lines[0])["record"]["message"] == "should appear"


def test_level_comes_from_settings(monkeypatch, capsys):
    import tshark_shared.config as config_module
    import tshark_shared.logging as logging_module

    monkeypatch.setenv("LOG_LEVEL", "ERROR")
    import importlib

    importlib.reload(config_module)
    importlib.reload(logging_module)
    try:
        logging_module.configure_logging("unit-test")
        logging_module.logger.warning("filtered by ERROR threshold")
        logging_module.logger.error("passes ERROR threshold")

        lines = [line for line in capsys.readouterr().out.splitlines() if line.strip()]
        assert len(lines) == 1
        assert json.loads(lines[0])["record"]["level"]["name"] == "ERROR"
    finally:
        monkeypatch.undo()
        importlib.reload(config_module)
        importlib.reload(logging_module)


def test_reconfiguring_does_not_duplicate_output(capsys):
    configure_logging("unit-test")
    configure_logging("unit-test")
    logger.info("once")

    lines = [line for line in capsys.readouterr().out.splitlines() if line.strip()]
    assert len(lines) == 1
