"""WSGI application entry point."""

from shop.settings import config


def get_wsgi_application():
    """Create and return the WSGI application callable."""
    debug = config.get("DEBUG", False)
    if debug:
        print("Running in debug mode")
    return application


def application(environ, start_response):
    """WSGI application callable."""
    status = "200 OK"
    headers = [("Content-Type", "text/html")]
    start_response(status, headers)
    return [b"Shop Application"]
