"""Main API module."""

from mylib.models import User, Item
from mylib.utils import format_response, sanitize_input
from mylib.auth.login import authenticate
from mylib.middleware import AuthMiddleware


def create_app(config=None):
    """Create and configure the application."""
    app = {"routes": {}, "middleware": []}
    if config:
        app["config"] = config
    app["middleware"].append(AuthMiddleware(app))
    return app


def get_users():
    """List all users."""
    users = [User("alice", "alice@example.com"), User("bob", "bob@example.com")]
    return format_response([u.to_dict() for u in users])


def create_user(data):
    """Create a new user."""
    username = sanitize_input(data.get("username"))
    email = sanitize_input(data.get("email"))
    user = User(username, email)
    if not user.validate():
        return format_response({"error": "invalid"}, status=400)
    user.save()
    return format_response(user.to_dict(), status=201)


def login(data):
    """Authenticate a user."""
    token = authenticate(data.get("username"), data.get("password"))
    if token:
        return format_response({"token": token})
    return format_response({"error": "unauthorized"}, status=401)
