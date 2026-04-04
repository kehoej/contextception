"""Quickstart guide examples."""
from webapp import settings
from webapp.core.base import Application

def basic_setup():
    """Basic application setup."""
    app = Application(settings)
    app.setup()
    return app

def configure_settings():
    """Configure application settings."""
    settings.DEBUG = True
    settings.DATABASE_URL = 'sqlite:///dev.db'

def run_example():
    """Run example application."""
    app = basic_setup()
    # Application logic here
    pass

if __name__ == '__main__':
    run_example()
