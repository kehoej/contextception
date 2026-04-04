"""Database connection management."""

from mylib.config import Config

_connection = None


def get_connection():
    """Get or create a database connection."""
    global _connection
    if _connection is None:
        _connection = _create_connection(Config.DATABASE_URL)
    return _connection


def close_connection():
    """Close the active database connection."""
    global _connection
    if _connection is not None:
        _connection = None


def _create_connection(url):
    """Create a new database connection from URL."""
    return {"url": url, "connected": True}
