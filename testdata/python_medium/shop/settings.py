"""Application configuration and settings."""

import os

config = {
    "DEBUG": os.environ.get("SHOP_DEBUG", "false").lower() == "true",
    "DATABASE_URL": os.environ.get("DATABASE_URL", "sqlite:///shop.db"),
    "SECRET_KEY": os.environ.get("SECRET_KEY", "dev-secret-key"),
    "HOST": os.environ.get("SHOP_HOST", "127.0.0.1"),
    "PORT": int(os.environ.get("SHOP_PORT", "8000")),
    "ALLOWED_HOSTS": os.environ.get("ALLOWED_HOSTS", "*").split(","),
    "CACHE_TTL": int(os.environ.get("CACHE_TTL", "300")),
    "LOG_LEVEL": os.environ.get("LOG_LEVEL", "INFO"),
}

INSTALLED_APPS = [
    "shop.accounts",
    "shop.products",
    "shop.orders",
]


def get_database_url():
    """Return the configured database URL."""
    return config["DATABASE_URL"]


def is_debug():
    """Return whether debug mode is enabled."""
    return config["DEBUG"]
