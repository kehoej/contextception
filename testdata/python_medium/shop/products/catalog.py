"""Catalog browsing and product listing features."""

from shop.products.models import Product, Category
from shop.products.services import list_products


def get_catalog(category=None):
    """Return all products, optionally filtered by category."""
    products = list_products(available_only=True)
    if category:
        products = [p for p in products if p.category and p.category.id == category.id]
    return products


def get_featured_products(limit=10):
    """Return a list of featured products for the homepage."""
    products = list_products(available_only=True)
    return sorted(products, key=lambda p: p.created_at, reverse=True)[:limit]


def get_categories():
    """Return all product categories."""
    products = list_products(available_only=False)
    categories = set()
    for product in products:
        if product.category:
            categories.add(product.category)
    return list(categories)


def get_products_by_price_range(min_price, max_price):
    """Return products within a given price range."""
    products = list_products(available_only=True)
    return [p for p in products if min_price <= p.price <= max_price]
