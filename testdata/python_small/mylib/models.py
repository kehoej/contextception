"""Core data models."""

from mylib.base import BaseModel
from mylib.db.connection import get_connection


class User(BaseModel):
    """User model."""

    def __init__(self, username, email):
        super().__init__()
        self.username = username
        self.email = email

    def validate(self):
        return bool(self.username and self.email)

    def to_dict(self):
        return {"username": self.username, "email": self.email}

    def save(self):
        conn = get_connection()
        conn.execute("INSERT INTO users VALUES (?, ?)", (self.username, self.email))


class Item(BaseModel):
    """Item model."""

    def __init__(self, name, price):
        super().__init__()
        self.name = name
        self.price = price

    def validate(self):
        return self.price > 0

    def to_dict(self):
        return {"name": self.name, "price": self.price}
