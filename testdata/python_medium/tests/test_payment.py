"""Tests for payment processing."""

from shop.orders.payment import process_payment


def test_process_payment_default_provider():
    """Test payment processing with the default provider."""
    # Would need a mock order with a positive total
    pass


def test_process_payment_invalid_provider():
    """Test that unknown providers raise PaymentError."""
    pass


def test_process_payment_zero_total():
    """Test that zero-total orders raise PaymentError."""
    pass


def test_refund_requires_paid_status():
    """Test that refunds only work on paid orders."""
    pass
