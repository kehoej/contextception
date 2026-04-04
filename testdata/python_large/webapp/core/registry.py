"""Component registry for dependency injection."""
from webapp.core.base import BaseComponent
from webapp import settings

class Registry(BaseComponent):
    """Registry for managing components."""

    def __init__(self):
        super().__init__()
        self._components = {}

    def register(self, name, component):
        """Register a component by name."""
        self._components[name] = component

    def get(self, name):
        """Retrieve a registered component."""
        return self._components.get(name)
