"""Application configuration."""

import os


class Config:
    """Base configuration."""

    DEBUG = False
    SECRET_KEY = os.environ.get("SECRET_KEY", "dev-key")
    DATABASE_URL = os.environ.get("DATABASE_URL", "sqlite:///app.db")
    LOG_LEVEL = os.environ.get("LOG_LEVEL", "INFO")


class DevelopmentConfig(Config):
    DEBUG = True


class ProductionConfig(Config):
    DEBUG = False
