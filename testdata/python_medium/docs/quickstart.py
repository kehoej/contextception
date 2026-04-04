"""Quickstart guide: creating users and browsing products."""

from shop.accounts.models import User
from shop.products.models import Product


def quickstart_example():
    """Demonstrate basic shop operations."""
    # Create a user
    user = User(username="demo_user", email="demo@example.com")
    print(f"Created user: {user.username}")

    # Create a product
    product = Product(name="Demo Widget", price=19.99)
    print(f"Created product: {product.name} at ${product.price}")

    # Check availability
    product.update_stock(5)
    print(f"In stock: {product.is_available}")

    return user, product


if __name__ == "__main__":
    quickstart_example()
