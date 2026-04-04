"""Tests for the API module."""

from mylib.api import create_app, get_users, create_user, login


def test_create_app():
    app = create_app()
    assert "routes" in app
    assert "middleware" in app


def test_get_users():
    result = get_users()
    assert result["status"] == 200
    assert len(result["data"]) == 2


def test_create_user():
    data = {"username": "test", "email": "test@example.com"}
    result = create_user(data)
    assert result["status"] == 201


def test_login():
    data = {"username": "alice", "password": "secret"}
    result = login(data)
    assert result["status"] == 200
