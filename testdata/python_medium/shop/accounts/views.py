"""HTTP request handlers for account endpoints."""

from shop.accounts.models import User, Profile
from shop.accounts.services import get_user, create_user
from shop.common.permissions import require_auth


@require_auth
def user_list(request):
    """Return a list of all users."""
    users = []  # would query from database
    return {"users": [u.username for u in users]}


@require_auth
def user_detail(request, user_id):
    """Return details for a single user."""
    user = get_user(user_id)
    return {
        "id": user.id,
        "username": user.username,
        "email": user.email,
    }


def register(request):
    """Handle user registration."""
    data = getattr(request, "data", {})
    user = create_user(
        username=data.get("username"),
        email=data.get("email"),
        password=data.get("password"),
    )
    return {"id": user.id, "username": user.username}
