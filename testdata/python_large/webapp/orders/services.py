"""Order business logic."""
from webapp.orders.models import Order
from webapp.auth.models import User
from webapp.catalog.models import Product
from webapp.orders.payment import PaymentProcessor
from webapp.orders.notifications import send_order_notification

class OrderService:
    """Order management service."""

    def __init__(self):
        self.payment_processor = PaymentProcessor()

    def create_order(self, user, items):
        """Create new order."""
        order = Order(user.id, items)
        order.calculate_total()
        order.save()
        send_order_notification(order)
        return order

    def process_payment(self, order, payment_method):
        """Process payment for order."""
        result = self.payment_processor.charge(order, payment_method)
        if result['success']:
            order.status = 'paid'
            order.save()
        return result
