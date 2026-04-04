"""Permission and role checking."""

from mylib.config import Config

ROLES = {
    "admin": ["read", "write", "delete"],
    "editor": ["read", "write"],
    "viewer": ["read"],
}


def check_role(user, role):
    """Check if a user has the given role."""
    return role in ROLES


def require_permission(request, permission):
    """Check that the request has the required permission."""
    role = request.get("role", "viewer")
    permissions = ROLES.get(role, [])
    if permission not in permissions:
        return {"error": "forbidden", "status": 403}
    return None


def get_permissions(role):
    """Get all permissions for a role."""
    return ROLES.get(role, [])
