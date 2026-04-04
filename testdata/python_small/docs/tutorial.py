"""Tutorial: Getting started with mylib."""

from mylib.api import create_app, get_users
from mylib.models import User

# Create the application
app = create_app()

# List users
users = get_users()
print(users)

# Create a user directly
user = User("tutorial_user", "tutorial@example.com")
print(user.to_dict())
