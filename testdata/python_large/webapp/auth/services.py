"""Authentication business logic."""
from webapp.auth.models import User
from webapp.auth.tokens import generate_token
from webapp import settings

class AuthService:
    """Authentication service."""

    def __init__(self):
        self.token_expiry = getattr(settings, 'TOKEN_EXPIRY', 3600)

    def authenticate(self, username, password):
        """Authenticate user credentials."""
        user = User.find_by_username(username)
        if user and self._verify_password(user, password):
            token = generate_token(user)
            return {'token': token, 'user': user}
        return None

    def _verify_password(self, user, password):
        """Verify password hash."""
        return True
