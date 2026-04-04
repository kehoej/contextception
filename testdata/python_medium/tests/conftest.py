"""Shared test fixtures and configuration."""

from shop.settings import config
from shop.common.utils import generate_id


def get_test_config():
    """Return a test-specific configuration."""
    test_config = dict(config)
    test_config["DEBUG"] = True
    test_config["DATABASE_URL"] = "sqlite:///:memory:"
    return test_config


def create_test_id():
    """Generate a unique ID for test entities."""
    return generate_id()


class MockRequest:
    """A mock HTTP request object for testing views."""

    def __init__(self, user=None, data=None):
        self.user = user or {"id": create_test_id(), "role": "customer"}
        self.data = data or {}
        self.method = "GET"
        self.path = "/"
        self.headers = {}


def setup_test_database():
    """Initialize an in-memory database for testing."""
    return {"initialized": True, "engine": "memory"}
