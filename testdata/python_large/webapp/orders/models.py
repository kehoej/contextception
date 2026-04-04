"""Order models."""
from webapp.core.base import BaseComponent
from webapp.db.connection import get_connection

class Order(BaseComponent):
    """Order model."""

    def __init__(self, user_id, items=None):
        super().__init__()
        self.user_id = user_id
        self.items = items or []
        self.total = 0
        self.status = 'pending'
        self.id = None

    def save(self):
        """Persist order to database."""
        conn = get_connection()
        # Save logic here
        pass

    def calculate_total(self):
        """Calculate order total."""
        self.total = sum(item['price'] * item['quantity'] for item in self.items)
        return self.total

class OrderItem(BaseComponent):
    """Order line item."""

    def __init__(self, product_id, quantity, price):
        super().__init__()
        self.product_id = product_id
        self.quantity = quantity
        self.price = price
