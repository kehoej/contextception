"""Product API endpoints."""
from webapp.catalog.models import Product
from webapp.api.serializers import BaseSerializer
from webapp.api.errors import APIError

def list_products(request):
    """List all products."""
    page = request.args.get('page', 1)
    # Query products
    return {'products': [], 'page': page}

def get_product(request, product_id):
    """Get product by ID."""
    serializer = BaseSerializer()
    # Find product
    return {'product': None}

def create_product(request):
    """Create new product."""
    product_data = request.json
    if not product_data.get('name'):
        raise APIError('Product name required')
    return {'product': product_data}
