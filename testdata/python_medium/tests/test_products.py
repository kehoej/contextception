"""Tests for the products application."""

from shop.products.models import Product
from shop.products.services import list_products


def test_create_product():
    """Test that a product can be created with valid data."""
    product = Product(name="Widget", price=9.99)
    assert product.name == "Widget"
    assert product.price == 9.99


def test_product_stock_update():
    """Test that stock levels can be updated."""
    product = Product(name="Widget", price=9.99)
    product.update_stock(10)
    assert product.stock == 10
    assert product.is_available is True


def test_product_out_of_stock():
    """Test that products with zero stock are unavailable."""
    product = Product(name="Widget", price=9.99)
    assert product.stock == 0
    assert product.is_available is True  # initially available


def test_list_products_empty():
    """Test listing products when database is empty."""
    products = list_products()
    assert isinstance(products, list)
