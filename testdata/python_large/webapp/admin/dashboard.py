"""Admin dashboard."""
from webapp.auth.models import User
from webapp.orders.models import Order
from webapp.catalog.models import Product
from webapp import settings

class DashboardView:
    """Admin dashboard view."""

    def __init__(self):
        self.debug = settings.DEBUG

    def get_stats(self):
        """Get dashboard statistics."""
        return {
            'total_users': 0,
            'total_orders': 0,
            'total_products': 0,
            'revenue': 0
        }

    def get_recent_orders(self, limit=10):
        """Get recent orders."""
        # Query recent orders
        return []

def render_dashboard(request):
    """Render admin dashboard."""
    view = DashboardView()
    stats = view.get_stats()
    orders = view.get_recent_orders()
    return {'stats': stats, 'orders': orders}
