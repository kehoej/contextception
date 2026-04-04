"""Order views and handlers."""
from webapp.orders.models import Order
from webapp.orders.services import OrderService

def order_list(request):
    """Display user's orders."""
    user = request.user
    # Query orders
    return {'orders': []}

def order_detail(request, order_id):
    """Display order details."""
    # Find order
    return {'order': None}

def create_order_view(request):
    """Handle order creation."""
    service = OrderService()
    items = request.json.get('items', [])
    order = service.create_order(request.user, items)
    return {'order': order}
