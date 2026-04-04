"""Product data models for catalog items and categories."""

from shop.common.utils import generate_id
from shop.common.validators import validate_price

from datetime import datetime


class Category:
    """A product category for organizing items."""

    def __init__(self, name, description=""):
        self.id = generate_id()
        self.name = name
        self.description = description
        self.parent = None

    def set_parent(self, parent_category):
        """Set this category's parent for hierarchy."""
        self.parent = parent_category

    def __repr__(self):
        return f"<Category {self.name}>"


class Product:
    """A purchasable item in the shop catalog."""

    def __init__(self, name, price, category=None):
        self.id = generate_id()
        self.name = name
        self.price = validate_price(price)
        self.category = category
        self.description = ""
        self.stock = 0
        self.is_available = True
        self.created_at = datetime.utcnow()

    def update_stock(self, quantity):
        """Adjust the stock level by a given quantity."""
        self.stock += quantity
        self.is_available = self.stock > 0

    def set_price(self, new_price):
        """Update the product price with validation."""
        self.price = validate_price(new_price)

    def __repr__(self):
        return f"<Product {self.name} ${self.price}>"
