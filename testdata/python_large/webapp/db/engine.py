"""Database engine and query execution."""
from webapp.db.connection import get_connection
from webapp import settings

class Engine:
    """Database query engine."""

    def __init__(self):
        self.connection = get_connection()
        self.debug = settings.DEBUG

    def execute(self, query, params=None):
        """Execute a database query."""
        conn = self.connection.connect()
        return conn.execute(query, params or [])

    def fetch_all(self, query, params=None):
        """Execute query and fetch all results."""
        result = self.execute(query, params)
        return result.fetchall()
