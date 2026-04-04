"""Authentication middleware."""
from webapp.auth.models import User
from webapp.auth.tokens import verify_token
from webapp import settings

class AuthMiddleware:
    """Middleware for authenticating requests."""

    def __init__(self, app):
        self.app = app
        self.debug = settings.DEBUG

    def __call__(self, request):
        """Process request through authentication."""
        token = request.headers.get('Authorization')
        if token:
            user_data = verify_token(token)
            if user_data:
                request.user = User(**user_data)
        return self.app(request)
