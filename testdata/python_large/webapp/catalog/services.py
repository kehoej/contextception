"""Catalog business logic."""
from webapp.catalog.models import Product
from webapp.catalog.pricing import PricingEngine
from webapp.catalog.inventory import InventoryManager

class CatalogService:
    """Catalog management service."""

    def __init__(self):
        self.pricing = PricingEngine()
        self.inventory = InventoryManager()

    def get_product_with_price(self, product_id):
        """Get product with calculated price."""
        product = Product.find_by_sku(product_id)
        if product:
            product.price = self.pricing.calculate_price(product)
        return product

    def check_availability(self, product_id, quantity):
        """Check if product is available."""
        return self.inventory.check_stock(product_id, quantity)
