"""Application-wide exception classes."""


class ShopError(Exception):
    """Base exception for all shop errors."""

    def __init__(self, message="An error occurred", code=None):
        self.message = message
        self.code = code
        super().__init__(self.message)


class NotFoundError(ShopError):
    """Raised when a requested resource is not found."""

    def __init__(self, resource="Resource", identifier=None):
        message = f"{resource} not found"
        if identifier:
            message = f"{resource} with id '{identifier}' not found"
        super().__init__(message, code="NOT_FOUND")


class ValidationError(ShopError):
    """Raised when input validation fails."""

    def __init__(self, field=None, message="Validation failed"):
        self.field = field
        super().__init__(message, code="VALIDATION_ERROR")


class PaymentError(ShopError):
    """Raised when payment processing fails."""

    def __init__(self, message="Payment processing failed", provider=None):
        self.provider = provider
        super().__init__(message, code="PAYMENT_ERROR")


class AuthenticationError(ShopError):
    """Raised when authentication fails."""

    def __init__(self, message="Authentication required"):
        super().__init__(message, code="AUTH_ERROR")
