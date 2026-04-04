"""Application settings and configuration."""
import os
import json
import logging

DEBUG = os.getenv('DEBUG', 'false').lower() == 'true'
SECRET_KEY = os.getenv('SECRET_KEY', 'dev-secret-key')
DATABASE_URL = os.getenv('DATABASE_URL', 'sqlite:///app.db')

LOGGING_LEVEL = logging.INFO if not DEBUG else logging.DEBUG

def load_config(path):
    """Load configuration from JSON file."""
    with open(path) as f:
        return json.load(f)
