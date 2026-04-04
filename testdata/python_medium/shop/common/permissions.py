"""Permission checks and authorization utilities."""

from shop.settings import config


def require_auth(func):
    """Decorator that enforces authentication on a view."""
    def wrapper(request, *args, **kwargs):
        if not getattr(request, "user", None):
            raise PermissionError("Authentication required")
        return func(request, *args, **kwargs)
    return wrapper


def require_role(role):
    """Decorator that enforces a specific user role."""
    def decorator(func):
        def wrapper(request, *args, **kwargs):
            user_role = getattr(request, "role", None)
            if user_role != role:
                raise PermissionError(f"Role '{role}' required")
            return func(request, *args, **kwargs)
        return wrapper
    return decorator


def is_admin(user):
    """Check whether a user has admin privileges."""
    return getattr(user, "role", None) == "admin"


def check_debug_access():
    """Allow unrestricted access in debug mode."""
    return config.get("DEBUG", False)
