"""Business logic for account operations."""

from shop.accounts.models import User
from shop.common.utils import generate_id
from shop.common.exceptions import NotFoundError

_users_db = {}


def create_user(username, email, password=None):
    """Create a new user and store in the database."""
    user = User(username=username, email=email)
    _users_db[user.id] = user
    return user


def get_user(user_id):
    """Retrieve a user by ID or raise NotFoundError."""
    user = _users_db.get(user_id)
    if user is None:
        raise NotFoundError("User", user_id)
    return user


def list_users(active_only=True):
    """Return all users, optionally filtered by active status."""
    users = list(_users_db.values())
    if active_only:
        users = [u for u in users if u.is_active]
    return users


def delete_user(user_id):
    """Deactivate a user by ID."""
    user = get_user(user_id)
    user.deactivate()
    return user
