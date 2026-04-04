"""Authentication models."""
from webapp.core.base import BaseComponent
from webapp.db.connection import get_connection

class User(BaseComponent):
    """User model."""

    def __init__(self, username, email, password_hash=None):
        super().__init__()
        self.username = username
        self.email = email
        self.password_hash = password_hash
        self.id = None

    def save(self):
        """Persist user to database."""
        conn = get_connection()
        # Save logic here
        pass

    @classmethod
    def find_by_username(cls, username):
        """Find user by username."""
        conn = get_connection()
        # Query logic here
        return None
