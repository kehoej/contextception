"""Form validation for account-related user input."""

from shop.accounts.models import User
from shop.common.validators import validate_email, validate_password


class RegistrationForm:
    """Validates user registration data."""

    def __init__(self, data):
        self.data = data
        self.errors = []

    def validate(self):
        """Run all validation checks and return True if valid."""
        self.errors = []
        try:
            validate_email(self.data.get("email", ""))
        except Exception as exc:
            self.errors.append(str(exc))
        try:
            validate_password(self.data.get("password", ""))
        except Exception as exc:
            self.errors.append(str(exc))
        if not self.data.get("username"):
            self.errors.append("Username is required")
        return len(self.errors) == 0

    def get_cleaned_data(self):
        """Return validated and cleaned form data."""
        return {
            "username": self.data["username"].strip(),
            "email": self.data["email"].strip().lower(),
            "password": self.data["password"],
        }


class ProfileForm:
    """Validates profile update data."""

    def __init__(self, data):
        self.data = data
        self.errors = []

    def validate(self):
        """Run validation checks on profile data."""
        self.errors = []
        display_name = self.data.get("display_name", "")
        if display_name and len(display_name) > 100:
            self.errors.append("Display name too long")
        return len(self.errors) == 0
