"""Serialization logic for product data models."""

from shop.products.models import Product, Category


def serialize_product(product):
    """Convert a Product instance to a dictionary."""
    return {
        "id": product.id,
        "name": product.name,
        "price": product.price,
        "stock": product.stock,
        "is_available": product.is_available,
        "category": serialize_category(product.category) if product.category else None,
    }


def serialize_category(category):
    """Convert a Category instance to a dictionary."""
    return {
        "id": category.id,
        "name": category.name,
        "description": category.description,
    }


def deserialize_product(data):
    """Create a Product instance from a dictionary."""
    return Product(
        name=data["name"],
        price=data["price"],
    )


def serialize_product_list(products):
    """Serialize a list of Product instances."""
    return [serialize_product(p) for p in products]
