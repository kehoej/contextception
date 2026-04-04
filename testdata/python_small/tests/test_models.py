"""Tests for the models module."""

from mylib.models import User, Item


def test_user_validate():
    user = User("alice", "alice@example.com")
    assert user.validate()


def test_user_to_dict():
    user = User("bob", "bob@example.com")
    d = user.to_dict()
    assert d["username"] == "bob"


def test_item_validate():
    item = Item("Widget", 9.99)
    assert item.validate()


def test_item_invalid_price():
    item = Item("Widget", -1)
    assert not item.validate()
