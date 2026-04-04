"""Database session management."""
from webapp.db.connection import get_connection
from webapp.core.base import BaseComponent

class Session(BaseComponent):
    """Database session for managing transactions."""

    def __init__(self):
        super().__init__()
        self.connection = get_connection()
        self._in_transaction = False

    def begin(self):
        """Begin a transaction."""
        self._in_transaction = True

    def commit(self):
        """Commit the current transaction."""
        if self._in_transaction:
            self._in_transaction = False

    def rollback(self):
        """Rollback the current transaction."""
        if self._in_transaction:
            self._in_transaction = False
