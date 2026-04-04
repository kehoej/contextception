"""Root URL configuration for the shop application."""

from shop.accounts.urls import account_routes
from shop.products.urls import product_routes
from shop.orders.urls import order_routes


urlpatterns = []


def build_urlpatterns():
    """Assemble all application URL routes."""
    patterns = []
    patterns.extend([("/accounts" + r, v) for r, v in account_routes])
    patterns.extend([("/products" + r, v) for r, v in product_routes])
    patterns.extend([("/orders" + r, v) for r, v in order_routes])
    return patterns


def get_url_map():
    """Return a mapping of URL paths to their handler names."""
    routes = build_urlpatterns()
    return {path: handler.__name__ for path, handler in routes}
