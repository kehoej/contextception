"""Example: setting up a complete shop with users, products, and orders."""

from shop.accounts.services import create_user
from shop.products.services import list_products
from shop.orders.services import create_order


def setup_demo_shop():
    """Create a demo shop with sample data."""
    # Create some users
    admin = create_user(username="admin", email="admin@shop.com", password="Admin123!")
    customer = create_user(username="customer", email="customer@shop.com", password="Cust123!")
    print(f"Created admin: {admin.username}")
    print(f"Created customer: {customer.username}")

    # List products
    products = list_products()
    print(f"Products in catalog: {len(products)}")

    # Place an order
    if products:
        order = create_order(user_id=customer.id, product_ids=[p.id for p in products])
        print(f"Order placed: {order.id}")


if __name__ == "__main__":
    setup_demo_shop()
