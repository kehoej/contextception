"""WSGI application entry point."""
from webapp import settings
from webapp.core.base import Application

application = Application(settings)

def get_wsgi_application():
    """Return the WSGI application instance."""
    return application
