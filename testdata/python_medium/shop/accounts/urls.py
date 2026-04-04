"""URL route definitions for account endpoints."""

from shop.accounts.views import user_list, user_detail

account_routes = [
    ("/", user_list),
    ("/<user_id>", user_detail),
]


def get_account_routes():
    """Return the list of account URL routes."""
    return account_routes
