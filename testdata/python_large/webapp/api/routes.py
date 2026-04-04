"""API route configuration."""
from webapp.api.v1 import users
from webapp.api.v1 import products
from webapp.api.v1 import orders
from webapp.api.v1 import search
from webapp.api.middleware import api_auth

class Router:
    """API route manager."""

    def __init__(self):
        self.routes = {}
        self.middleware = [api_auth]
        self._register_routes()

    def _register_routes(self):
        """Register all API routes."""
        self.routes['/api/v1/users'] = users
        self.routes['/api/v1/products'] = products
        self.routes['/api/v1/orders'] = orders
        self.routes['/api/v1/search'] = search

    def route(self, path):
        """Get handler for path."""
        return self.routes.get(path)

router = Router()
