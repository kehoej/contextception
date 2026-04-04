"""Catalog models."""
from webapp.core.base import BaseComponent
from webapp.db.connection import get_connection

class Product(BaseComponent):
    """Product model."""

    def __init__(self, name, price, sku=None):
        super().__init__()
        self.name = name
        self.price = price
        self.sku = sku
        self.id = None

    def save(self):
        """Persist product to database."""
        conn = get_connection()
        # Save logic here
        pass

    @classmethod
    def find_by_sku(cls, sku):
        """Find product by SKU."""
        conn = get_connection()
        # Query logic here
        return None

class Category(BaseComponent):
    """Product category."""

    def __init__(self, name, parent=None):
        super().__init__()
        self.name = name
        self.parent = parent
