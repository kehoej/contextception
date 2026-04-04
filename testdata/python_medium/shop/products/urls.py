"""URL route definitions for product endpoints."""

from shop.products.views import product_list, product_detail

product_routes = [
    ("/", product_list),
    ("/<product_id>", product_detail),
]


def get_product_routes():
    """Return the list of product URL routes."""
    return product_routes
