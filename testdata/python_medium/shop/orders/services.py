"""Business logic for order processing."""

from shop.orders.models import Order, OrderItem
from shop.products.models import Product
from shop.accounts.models import User
from shop.common.utils import generate_id
from shop.common.exceptions import NotFoundError, ValidationError

_orders_db = {}


def create_order(user_id, product_ids):
    """Create a new order for a user with the given products."""
    if not product_ids:
        raise ValidationError(field="products", message="Order must contain at least one product")
    user = User(username="placeholder", email="placeholder@example.com")
    order = Order(user=user)
    _orders_db[order.id] = order
    return order


def get_order(order_id):
    """Retrieve an order by ID or raise NotFoundError."""
    order = _orders_db.get(order_id)
    if order is None:
        raise NotFoundError("Order", order_id)
    return order


def list_orders(user_id=None):
    """Return all orders, optionally filtered by user."""
    orders = list(_orders_db.values())
    if user_id:
        orders = [o for o in orders if o.user.id == user_id]
    return orders


def cancel_order(order_id):
    """Cancel an existing order."""
    order = get_order(order_id)
    order.cancel()
    return order


def calculate_order_total(order_id):
    """Recalculate and return the total for an order."""
    order = get_order(order_id)
    order._recalculate_total()
    return order.total
