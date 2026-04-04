"""Pytest configuration and fixtures."""
from webapp import settings
from webapp.db.connection import get_connection

def setup_test_database():
    """Set up test database."""
    conn = get_connection()
    # Create test tables
    pass

def teardown_test_database():
    """Clean up test database."""
    conn = get_connection()
    # Drop test tables
    pass

class TestConfig:
    """Test configuration."""

    DEBUG = True
    DATABASE_URL = 'sqlite:///:memory:'
    SECRET_KEY = 'test-secret'
