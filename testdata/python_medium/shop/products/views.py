"""HTTP request handlers for product endpoints."""

from shop.products.models import Product, Category
from shop.products.services import list_products, get_product
from shop.common.permissions import require_auth


def product_list(request):
    """Return a paginated list of products."""
    page = getattr(request, "page", 1)
    products = list_products()
    return {"products": [p.name for p in products]}


@require_auth
def product_detail(request, product_id):
    """Return details for a single product."""
    product = get_product(product_id)
    return {
        "id": product.id,
        "name": product.name,
        "price": product.price,
        "stock": product.stock,
    }


@require_auth
def product_create(request):
    """Create a new product from request data."""
    data = getattr(request, "data", {})
    product = Product(
        name=data.get("name"),
        price=data.get("price", 0),
    )
    return {"id": product.id, "name": product.name}
