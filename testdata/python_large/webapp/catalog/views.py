"""Catalog views and handlers."""
from webapp.catalog.models import Product
from webapp.catalog.services import CatalogService

def product_list(request):
    """Display product listing."""
    service = CatalogService()
    # Get products
    return {'products': []}

def product_detail(request, product_id):
    """Display product details."""
    service = CatalogService()
    product = service.get_product_with_price(product_id)
    return {'product': product}
