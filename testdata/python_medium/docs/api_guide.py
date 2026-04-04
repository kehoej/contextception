"""API guide: working with orders and product services."""

from shop.orders.services import create_order
from shop.products.services import list_products


def api_usage_example():
    """Demonstrate API-level operations for orders and products."""
    # List available products
    products = list_products(available_only=True)
    print(f"Available products: {len(products)}")

    # Create an order
    if products:
        product_ids = [p.id for p in products[:3]]
        order = create_order(user_id="example-user", product_ids=product_ids)
        print(f"Order created: {order.id}")
        print(f"Order status: {order.status}")

    return products


if __name__ == "__main__":
    api_usage_example()
