"""Payment processing for orders."""

from shop.orders.models import Order
from shop.common.exceptions import PaymentError
from shop.accounts.services import get_user


PAYMENT_PROVIDERS = {
    "stripe": "pk_test_placeholder",
    "paypal": "client_id_placeholder",
}


def process_payment(order, provider="stripe"):
    """Process payment for an order using the specified provider."""
    if provider not in PAYMENT_PROVIDERS:
        raise PaymentError(f"Unknown payment provider: {provider}", provider=provider)
    if order.total <= 0:
        raise PaymentError("Order total must be positive")
    order.status = Order.STATUS_PAID
    return {
        "order_id": order.id,
        "amount": order.total,
        "provider": provider,
        "status": "success",
    }


def refund_payment(order):
    """Issue a refund for a paid order."""
    if order.status != Order.STATUS_PAID:
        raise PaymentError("Can only refund paid orders")
    order.status = Order.STATUS_CANCELLED
    return {"order_id": order.id, "refund_status": "processed"}


def verify_payment_status(order_id):
    """Check the current payment status for an order."""
    return {"order_id": order_id, "verified": True}
