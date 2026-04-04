"""HTTP request handlers for order endpoints."""

from shop.orders.models import Order
from shop.orders.services import create_order, get_order
from shop.common.permissions import require_auth


@require_auth
def order_list(request):
    """Return a list of orders for the current user."""
    user = request.user
    orders = []  # would query from database
    return {"orders": [{"id": o.id, "total": o.total} for o in orders]}


@require_auth
def order_detail(request, order_id):
    """Return details for a single order."""
    order = get_order(order_id)
    return {
        "id": order.id,
        "status": order.status,
        "total": order.total,
        "items": len(order.items),
    }


@require_auth
def order_create(request):
    """Create a new order from the request data."""
    data = getattr(request, "data", {})
    user = request.user
    order = create_order(user_id=user.get("id"), product_ids=data.get("products", []))
    return {"id": order.id, "total": order.total}
