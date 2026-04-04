"""Database query functions."""

from mylib.db.connection import get_connection
from mylib.models import User


def find_user(username):
    """Find a user by username."""
    conn = get_connection()
    row = conn.get("SELECT * FROM users WHERE username = ?", (username,))
    if row:
        return User(row["username"], row["email"])
    return None


def find_all_users():
    """Return all users."""
    conn = get_connection()
    rows = conn.get("SELECT * FROM users")
    return [User(r["username"], r["email"]) for r in rows]


def delete_user(username):
    """Delete a user by username."""
    conn = get_connection()
    conn.execute("DELETE FROM users WHERE username = ?", (username,))
