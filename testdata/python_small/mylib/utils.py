"""Shared utility functions."""

from mylib.config import Config


def get_logger(name):
    """Create a configured logger."""
    import logging
    logger = logging.getLogger(name)
    logger.setLevel(Config.LOG_LEVEL)
    return logger


def sanitize_input(value):
    """Sanitize user input."""
    if isinstance(value, str):
        return value.strip()
    return value


def format_response(data, status=200):
    """Format a standard API response."""
    return {"data": data, "status": status}
