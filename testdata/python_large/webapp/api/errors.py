"""API error handling."""
from webapp.core.base import BaseComponent

class APIError(BaseComponent, Exception):
    """Base API error class."""

    def __init__(self, message, status_code=400):
        super().__init__()
        self.message = message
        self.status_code = status_code

    def to_dict(self):
        """Convert error to dictionary."""
        return {
            'error': self.message,
            'status': self.status_code
        }

class ValidationError(APIError):
    """Validation error."""

    def __init__(self, message, field=None):
        super().__init__(message, status_code=422)
        self.field = field

class NotFoundError(APIError):
    """Resource not found error."""

    def __init__(self, resource_type):
        super().__init__(f"{resource_type} not found", status_code=404)
