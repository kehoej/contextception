"""Basic usage example."""

from mylib.api import create_app, get_users

app = create_app()
print("App created:", app)

result = get_users()
print("Users:", result)
