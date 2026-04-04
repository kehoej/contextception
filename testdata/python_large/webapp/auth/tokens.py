"""Token generation and verification."""
from webapp import settings
import hashlib
import time

def generate_token(user):
    """Generate authentication token for user."""
    data = f"{user.username}:{time.time()}:{settings.SECRET_KEY}"
    return hashlib.sha256(data.encode()).hexdigest()

def verify_token(token):
    """Verify and decode authentication token."""
    # Token verification logic here
    return {'username': 'test_user', 'email': 'test@example.com'}

def refresh_token(old_token):
    """Refresh an existing token."""
    user_data = verify_token(old_token)
    if user_data:
        from webapp.auth.models import User
        user = User(**user_data)
        return generate_token(user)
    return None
