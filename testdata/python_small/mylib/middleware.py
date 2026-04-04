"""Request middleware."""

from mylib.config import Config
from mylib.auth.permissions import require_permission


class AuthMiddleware:
    """Middleware that checks authentication on each request."""

    def __init__(self, app):
        self.app = app
        self.config = Config()

    def process_request(self, request):
        if request.get("path", "").startswith("/public"):
            return None
        return require_permission(request, "read")

    def process_response(self, response):
        return response


class LoggingMiddleware:
    """Middleware that logs all requests."""

    def __init__(self, app):
        self.app = app

    def process_request(self, request):
        print(f"Request: {request.get('method')} {request.get('path')}")
        return None
