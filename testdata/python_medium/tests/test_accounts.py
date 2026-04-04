"""Tests for the accounts application."""

from shop.accounts.models import User
from shop.accounts.services import create_user


def test_create_user():
    """Test that a user can be created with valid data."""
    user = create_user(username="testuser", email="test@example.com")
    assert user.username == "testuser"
    assert user.email == "test@example.com"
    assert user.is_active is True


def test_user_deactivate():
    """Test that a user can be deactivated."""
    user = User(username="testuser", email="test@example.com")
    user.deactivate()
    assert user.is_active is False


def test_user_has_id():
    """Test that created users have a unique identifier."""
    user = create_user(username="testuser", email="test@example.com")
    assert user.id is not None
    assert len(user.id) > 0


def test_user_default_role():
    """Test that new users get the default customer role."""
    user = User(username="newuser", email="new@example.com")
    assert user.role == "customer"
