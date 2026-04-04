"""Utility tests."""
from webapp.utils.formatting import format_json
from webapp.utils.validation import validate_email

def test_json_formatting():
    """Test JSON formatting."""
    data = {'key': 'value'}
    result = format_json(data)
    assert 'key' in result

def test_email_validation():
    """Test email validation."""
    assert validate_email('test@example.com') is True
    assert validate_email('invalid-email') is False

def test_currency_formatting():
    """Test currency formatting."""
    from webapp.utils.formatting import format_currency
    assert format_currency(10) == '$10.00'
    assert format_currency(10.5) == '$10.50'
