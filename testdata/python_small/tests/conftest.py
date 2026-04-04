"""Shared test fixtures."""

from mylib.db.connection import get_connection, close_connection
from mylib.config import Config


def setup_test_db():
    """Set up a test database."""
    Config.DATABASE_URL = "sqlite:///:memory:"
    conn = get_connection()
    return conn


def teardown_test_db():
    """Tear down the test database."""
    close_connection()
