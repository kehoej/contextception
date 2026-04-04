"""Tests for the orders application."""

from shop.orders.models import Order
from shop.orders.services import create_order


def test_create_order():
    """Test that an order can be created."""
    order = create_order(user_id="user-1", product_ids=["prod-1"])
    assert order.id is not None
    assert order.status == Order.STATUS_PENDING


def test_order_cancel():
    """Test that a pending order can be cancelled."""
    order = create_order(user_id="user-1", product_ids=["prod-1"])
    order.cancel()
    assert order.status == Order.STATUS_CANCELLED


def test_order_initial_total():
    """Test that a new order starts with zero total."""
    order = create_order(user_id="user-1", product_ids=["prod-1"])
    assert order.total == 0.0


def test_order_has_user():
    """Test that an order is associated with a user."""
    order = create_order(user_id="user-1", product_ids=["prod-1"])
    assert order.user is not None
