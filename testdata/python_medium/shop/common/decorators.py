"""Reusable function and method decorators."""

from shop.common.permissions import require_auth
from shop.common.utils import get_logger

import functools
import time

logger = get_logger(__name__)


def log_call(func):
    """Decorator that logs function entry and exit."""
    @functools.wraps(func)
    def wrapper(*args, **kwargs):
        logger.info(f"Calling {func.__name__}")
        result = func(*args, **kwargs)
        logger.info(f"Finished {func.__name__}")
        return result
    return wrapper


def retry(max_attempts=3, delay=1.0):
    """Decorator that retries a function on failure."""
    def decorator(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            last_error = None
            for attempt in range(max_attempts):
                try:
                    return func(*args, **kwargs)
                except Exception as exc:
                    last_error = exc
                    time.sleep(delay)
            raise last_error
        return wrapper
    return decorator


def deprecated(message="This function is deprecated"):
    """Mark a function as deprecated."""
    def decorator(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            logger.warning(f"DEPRECATED: {func.__name__} - {message}")
            return func(*args, **kwargs)
        return wrapper
    return decorator
