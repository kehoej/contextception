"""Permission checking and authorization."""
from webapp.auth.models import User
from webapp.core.base import BaseComponent

class Permission(BaseComponent):
    """Permission definition."""

    def __init__(self, name, description=''):
        super().__init__()
        self.name = name
        self.description = description

def check_permission(user, permission_name):
    """Check if user has permission."""
    if not isinstance(user, User):
        return False
    # Permission check logic here
    return True

def has_any_permission(user, *permission_names):
    """Check if user has any of the given permissions."""
    return any(check_permission(user, perm) for perm in permission_names)
