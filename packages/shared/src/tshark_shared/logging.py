"""Structured logging setup.

PRD §16.6 requires every service to emit JSON logs to stdout at the configured
LOG_LEVEL. This module previously configured loguru at import time with a
hardcoded INFO level and a human-readable format, so the LOG_LEVEL setting was
dead and the output was not machine-parseable.

Configuration is now explicit: services call configure_logging() at startup.
"""

import sys

from loguru import logger

from tshark_shared.config import settings

__all__ = ["logger", "configure_logging"]


def configure_logging(service: str, level: str | None = None) -> None:
    """Route loguru to stdout as JSON, tagged with the service name.

    Args:
        service: Service name bound to every record, so logs from the ingestor
            and the worker stay distinguishable once aggregated.
        level: Overrides LOG_LEVEL when given; used mainly by tests.
    """
    logger.remove()
    logger.add(
        sys.stdout,
        level=(level or settings.LOG_LEVEL).upper(),
        serialize=True,
        backtrace=False,
        diagnose=False,
    )
    logger.configure(extra={"service": service})
