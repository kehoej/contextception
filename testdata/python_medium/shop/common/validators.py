"""Input validation functions for common data types."""

from shop.common.exceptions import ValidationError

import re


def validate_email(email):
    """Validate an email address format."""
    pattern = r"^[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+$"
    if not re.match(pattern, email):
        raise ValidationError(field="email", message=f"Invalid email: {email}")
    return email


def validate_password(password):
    """Validate password meets minimum strength requirements."""
    if len(password) < 8:
        raise ValidationError(field="password", message="Password must be at least 8 characters")
    if not re.search(r"[A-Z]", password):
        raise ValidationError(field="password", message="Password must contain an uppercase letter")
    if not re.search(r"[0-9]", password):
        raise ValidationError(field="password", message="Password must contain a digit")
    return password


def validate_price(price):
    """Validate a price value is positive and reasonable."""
    if not isinstance(price, (int, float)):
        raise ValidationError(field="price", message="Price must be a number")
    if price < 0:
        raise ValidationError(field="price", message="Price cannot be negative")
    if price > 999999.99:
        raise ValidationError(field="price", message="Price exceeds maximum allowed")
    return round(price, 2)
