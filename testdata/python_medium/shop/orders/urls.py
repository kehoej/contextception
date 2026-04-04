"""URL route definitions for order endpoints."""

from shop.orders.views import order_list, order_detail

order_routes = [
    ("/", order_list),
    ("/<order_id>", order_detail),
]


def get_order_routes():
    """Return the list of order URL routes."""
    return order_routes
