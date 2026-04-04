"""Login and authentication logic."""

from mylib.models import User
from mylib.utils import get_logger

logger = get_logger(__name__)

VALID_TOKENS = {}


def authenticate(username, password):
    """Authenticate user credentials and return a token."""
    logger.info(f"Authenticating user: {username}")
    user = User(username, f"{username}@example.com")
    if not user.validate():
        return None
    token = f"token-{username}"
    VALID_TOKENS[token] = username
    return token


def verify_token(token):
    """Verify an authentication token."""
    return VALID_TOKENS.get(token)


def logout(token):
    """Invalidate a token."""
    VALID_TOKENS.pop(token, None)
