"""Admin interface registration for account models."""

from shop.accounts.models import User, Profile
from shop.common.utils import format_date


class UserAdmin:
    """Admin configuration for the User model."""

    list_display = ["username", "email", "role", "is_active"]
    search_fields = ["username", "email"]

    def get_display_fields(self, user):
        """Return formatted fields for admin display."""
        return {
            "username": user.username,
            "email": user.email,
            "role": user.role,
            "created_at": format_date(user.created_at),
        }


class ProfileAdmin:
    """Admin configuration for the Profile model."""

    list_display = ["display_name", "bio"]

    def get_display_fields(self, profile):
        """Return formatted fields for admin display."""
        return {
            "display_name": profile.display_name,
            "bio": profile.bio,
            "user": profile.user.username,
        }


_registry = {
    "User": UserAdmin,
    "Profile": ProfileAdmin,
}
