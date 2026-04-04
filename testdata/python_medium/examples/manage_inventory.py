"""Example: managing product inventory and catalog browsing."""

from shop.products.models import Product
from shop.products.catalog import get_catalog


def inventory_management_example():
    """Demonstrate inventory management operations."""
    # Create products
    widget = Product(name="Standard Widget", price=9.99)
    widget.update_stock(100)

    gadget = Product(name="Premium Gadget", price=49.99)
    gadget.update_stock(25)

    print(f"{widget.name}: {widget.stock} in stock")
    print(f"{gadget.name}: {gadget.stock} in stock")

    # Browse catalog
    catalog = get_catalog()
    print(f"Catalog contains {len(catalog)} products")

    # Simulate selling items
    widget.update_stock(-3)
    print(f"{widget.name}: {widget.stock} remaining after sale")


if __name__ == "__main__":
    inventory_management_example()
