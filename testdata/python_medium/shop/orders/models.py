"""Order data models for purchases and line items."""

from shop.products.models import Product
from shop.accounts.models import User
from shop.common.utils import generate_id

from datetime import datetime


class OrderItem:
    """A single line item within an order."""

    def __init__(self, product, quantity=1):
        self.id = generate_id()
        self.product = product
        self.quantity = quantity
        self.unit_price = product.price
        self.total = self.unit_price * self.quantity

    def __repr__(self):
        return f"<OrderItem {self.product.name} x{self.quantity}>"


class Order:
    """Represents a customer purchase order."""

    STATUS_PENDING = "pending"
    STATUS_PAID = "paid"
    STATUS_SHIPPED = "shipped"
    STATUS_DELIVERED = "delivered"
    STATUS_CANCELLED = "cancelled"

    def __init__(self, user):
        self.id = generate_id()
        self.user = user
        self.items = []
        self.status = self.STATUS_PENDING
        self.created_at = datetime.utcnow()
        self.total = 0.0

    def add_item(self, product, quantity=1):
        """Add a line item to the order."""
        item = OrderItem(product, quantity)
        self.items.append(item)
        self._recalculate_total()
        return item

    def _recalculate_total(self):
        """Recalculate the order total from line items."""
        self.total = sum(item.total for item in self.items)

    def cancel(self):
        """Cancel the order if it hasn't been shipped."""
        if self.status in (self.STATUS_SHIPPED, self.STATUS_DELIVERED):
            raise ValueError("Cannot cancel a shipped or delivered order")
        self.status = self.STATUS_CANCELLED

    def __repr__(self):
        return f"<Order {self.id[:8]} ${self.total}>"
