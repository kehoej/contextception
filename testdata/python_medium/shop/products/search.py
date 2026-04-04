"""Product search functionality."""

from shop.products.models import Product
from shop.common.utils import sanitize_input

_search_index = {}


def index_product(product):
    """Add a product to the search index."""
    terms = product.name.lower().split()
    for term in terms:
        if term not in _search_index:
            _search_index[term] = []
        _search_index[term].append(product.id)


def search_products(query, products_db=None):
    """Search for products matching a query string."""
    clean_query = sanitize_input(query).lower()
    terms = clean_query.split()
    matching_ids = set()
    for term in terms:
        matching_ids.update(_search_index.get(term, []))
    return list(matching_ids)


def clear_index():
    """Clear the search index."""
    _search_index.clear()


def rebuild_index(products):
    """Rebuild the search index from a list of products."""
    clear_index()
    for product in products:
        index_product(product)
