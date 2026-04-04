"""General-purpose utility functions used across the application."""

from shop.settings import config

import uuid
import logging
from datetime import datetime


def generate_id():
    """Generate a unique identifier string."""
    return str(uuid.uuid4())


def get_logger(name):
    """Create a configured logger instance."""
    logger = logging.getLogger(name)
    level = config.get("LOG_LEVEL", "INFO")
    logger.setLevel(getattr(logging, level))
    return logger


def format_date(dt):
    """Format a datetime object to ISO 8601 string."""
    if dt is None:
        return None
    return dt.isoformat()


def sanitize_input(text):
    """Remove potentially dangerous characters from input text."""
    if not isinstance(text, str):
        return str(text)
    return text.strip().replace("<", "&lt;").replace(">", "&gt;")


def paginate(items, page=1, per_page=20):
    """Return a paginated slice of items."""
    start = (page - 1) * per_page
    end = start + per_page
    return {
        "items": items[start:end],
        "page": page,
        "per_page": per_page,
        "total": len(items),
    }
