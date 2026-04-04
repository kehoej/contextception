"""Business logic for product operations."""

from shop.products.models import Product
from shop.common.utils import generate_id
from shop.common.exceptions import NotFoundError

_products_db = {}


def create_product(name, price, category=None):
    """Create a new product and store it."""
    product = Product(name=name, price=price, category=category)
    _products_db[product.id] = product
    return product


def get_product(product_id):
    """Retrieve a product by ID or raise NotFoundError."""
    product = _products_db.get(product_id)
    if product is None:
        raise NotFoundError("Product", product_id)
    return product


def list_products(available_only=True):
    """Return all products, optionally filtered by availability."""
    products = list(_products_db.values())
    if available_only:
        products = [p for p in products if p.is_available]
    return products


def update_stock(product_id, quantity):
    """Update the stock level for a product."""
    product = get_product(product_id)
    product.update_stock(quantity)
    return product


def delete_product(product_id):
    """Remove a product from the database."""
    if product_id not in _products_db:
        raise NotFoundError("Product", product_id)
    return _products_db.pop(product_id)
