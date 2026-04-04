"""Database connection management."""
from webapp import settings
from webapp.core.base import BaseComponent

class Connection(BaseComponent):
    """Database connection wrapper."""

    def __init__(self):
        super().__init__()
        self.url = settings.DATABASE_URL
        self._conn = None

    def connect(self):
        """Establish database connection."""
        if not self._conn:
            self._conn = self._create_connection()
        return self._conn

    def _create_connection(self):
        """Create the actual connection."""
        pass

_connection = None

def get_connection():
    """Get the global database connection."""
    global _connection
    if _connection is None:
        _connection = Connection()
    return _connection
