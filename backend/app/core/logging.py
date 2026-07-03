from __future__ import annotations

import logging
import sys

import structlog

from app.core.trace import bind_context

TRACE_LEVEL = 5


def _trace(self: logging.Logger, message: str, *args: object, **kwargs: object) -> None:
    if self.isEnabledFor(TRACE_LEVEL):
        self._log(TRACE_LEVEL, message, args, **kwargs)


logging.addLevelName(TRACE_LEVEL, "TRACE")
logging.Logger.trace = _trace  # type: ignore[attr-defined]


def configure_logging(level: str = "INFO") -> None:
    numeric_level = getattr(logging, level.upper(), logging.INFO)
    logging.basicConfig(format="%(message)s", stream=sys.stdout, level=numeric_level)
    structlog.configure(
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.stdlib.filter_by_level,
            structlog.stdlib.add_logger_name,
            structlog.stdlib.add_log_level,
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.format_exc_info,
            structlog.processors.UnicodeDecoder(),
            structlog.processors.JSONRenderer(),
        ],
        wrapper_class=structlog.stdlib.BoundLogger,
        logger_factory=structlog.stdlib.LoggerFactory(),
        cache_logger_on_first_use=True,
    )


def bind_trace_context(**kwargs: str) -> None:
    bind_context(**kwargs)
    structlog.contextvars.bind_contextvars(**{k: v for k, v in kwargs.items() if v is not None})
