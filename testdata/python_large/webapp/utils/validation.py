"""Data validation utilities."""
from webapp.core.base import BaseComponent
import re

class Validator(BaseComponent):
    """Data validator."""

    def __init__(self):
        super().__init__()
        self.errors = []

    def validate(self, data, rules):
        """Validate data against rules."""
        self.errors = []
        # Validation logic
        return len(self.errors) == 0

def validate_email(email):
    """Validate email address format."""
    pattern = r'^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$'
    return re.match(pattern, email) is not None

def validate_phone(phone):
    """Validate phone number format."""
    pattern = r'^\+?1?\d{10,15}$'
    return re.match(pattern, phone) is not None
