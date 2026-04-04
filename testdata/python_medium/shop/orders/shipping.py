"""Shipping and fulfillment logic for orders."""

from shop.orders.models import Order
from shop.common.utils import generate_id


SHIPPING_METHODS = {
    "standard": {"cost": 5.99, "days": 5},
    "express": {"cost": 12.99, "days": 2},
    "overnight": {"cost": 24.99, "days": 1},
}


def calculate_shipping(order, method="standard"):
    """Calculate shipping cost for an order."""
    if method not in SHIPPING_METHODS:
        raise ValueError(f"Unknown shipping method: {method}")
    return SHIPPING_METHODS[method]["cost"]


def create_shipment(order, method="standard"):
    """Create a shipment record for an order."""
    tracking_id = generate_id()
    shipping_info = SHIPPING_METHODS[method]
    order.status = Order.STATUS_SHIPPED
    return {
        "tracking_id": tracking_id,
        "method": method,
        "cost": shipping_info["cost"],
        "estimated_days": shipping_info["days"],
    }


def get_tracking_info(tracking_id):
    """Retrieve tracking information for a shipment."""
    return {
        "tracking_id": tracking_id,
        "status": "in_transit",
        "location": "Distribution Center",
    }
