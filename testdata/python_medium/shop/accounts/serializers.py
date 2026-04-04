"""Serialization logic for account data models."""

from shop.accounts.models import User, Profile


def serialize_user(user):
    """Convert a User instance to a dictionary."""
    return {
        "id": user.id,
        "username": user.username,
        "email": user.email,
        "role": user.role,
        "is_active": user.is_active,
    }


def serialize_profile(profile):
    """Convert a Profile instance to a dictionary."""
    return {
        "id": profile.id,
        "display_name": profile.display_name,
        "bio": profile.bio,
        "avatar_url": profile.avatar_url,
        "user": serialize_user(profile.user),
    }


def deserialize_user(data):
    """Create a User instance from a dictionary."""
    return User(
        username=data["username"],
        email=data["email"],
        role=data.get("role", "customer"),
    )


def serialize_user_list(users):
    """Serialize a list of User instances."""
    return [serialize_user(u) for u in users]
