"""API reference examples."""
from webapp.api.routes import router
from webapp.auth.models import User

def example_user_api():
    """Example: Using the User API."""
    # Create user
    user = User('example', 'example@test.com')

    # API endpoint usage
    route = router.route('/api/v1/users')
    # Make API call
    pass

def example_product_api():
    """Example: Using the Product API."""
    from webapp.catalog.models import Product

    product = Product('Example Product', 29.99)
    # API operations
    pass

def example_order_api():
    """Example: Using the Order API."""
    from webapp.orders.models import Order

    order = Order(user_id=1, items=[])
    # API operations
    pass
