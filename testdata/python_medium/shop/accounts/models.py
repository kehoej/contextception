"""Account data models for users and profiles."""

from shop.common.utils import generate_id
from shop.common.validators import validate_email

from datetime import datetime


class User:
    """Represents a registered user in the system."""

    def __init__(self, username, email, role="customer"):
        self.id = generate_id()
        self.username = username
        self.email = validate_email(email)
        self.role = role
        self.created_at = datetime.utcnow()
        self.is_active = True

    def deactivate(self):
        """Mark the user as inactive."""
        self.is_active = False

    def __repr__(self):
        return f"<User {self.username}>"


class Profile:
    """Extended profile information for a user."""

    def __init__(self, user, display_name=None, bio=""):
        self.id = generate_id()
        self.user = user
        self.display_name = display_name or user.username
        self.bio = bio
        self.avatar_url = None

    def update(self, display_name=None, bio=None):
        """Update profile fields."""
        if display_name is not None:
            self.display_name = display_name
        if bio is not None:
            self.bio = bio

    def __repr__(self):
        return f"<Profile {self.display_name}>"
