"""Request/response middleware components."""

from shop.common.permissions import require_auth
from shop.settings import config

import time


class LoggingMiddleware:
    """Middleware that logs request details."""

    def __init__(self, app):
        self.app = app
        self.debug = config.get("DEBUG", False)

    def __call__(self, request):
        start = time.time()
        response = self.app(request)
        duration = time.time() - start
        if self.debug:
            print(f"{request.method} {request.path} - {duration:.3f}s")
        return response


class AuthMiddleware:
    """Middleware that attaches user context to requests."""

    def __init__(self, app):
        self.app = app

    def __call__(self, request):
        token = getattr(request, "headers", {}).get("Authorization")
        if token:
            request.user = self._resolve_user(token)
        return self.app(request)

    def _resolve_user(self, token):
        """Look up a user from an auth token."""
        return {"token": token, "authenticated": True}
