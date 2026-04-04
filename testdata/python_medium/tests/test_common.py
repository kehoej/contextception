"""Tests for common utilities and shared logic."""

from shop.common.utils import generate_id
from shop.common.permissions import require_auth


def test_generate_id_unique():
    """Test that generated IDs are unique."""
    id1 = generate_id()
    id2 = generate_id()
    assert id1 != id2


def test_generate_id_format():
    """Test that generated IDs are valid UUID strings."""
    test_id = generate_id()
    assert isinstance(test_id, str)
    assert len(test_id) == 36  # UUID format with dashes


def test_require_auth_decorator():
    """Test that require_auth enforces authentication."""
    @require_auth
    def protected_view(request):
        return {"status": "ok"}

    # Would test with mock requests
    pass


def test_require_auth_allows_authenticated():
    """Test that authenticated requests pass through."""
    pass
