"""Logging configuration."""
from webapp import settings
import logging

def setup_logging():
    """Configure application logging."""
    logging.basicConfig(
        level=settings.LOGGING_LEVEL,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )

def get_logger(name):
    """Get logger instance."""
    return logging.getLogger(name)

class LogHandler:
    """Custom log handler."""

    def __init__(self, name):
        self.logger = get_logger(name)

    def log(self, level, message, **kwargs):
        """Log message with context."""
        self.logger.log(level, message, extra=kwargs)
