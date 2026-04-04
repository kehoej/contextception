"""Base classes for framework components."""
from webapp import settings

class BaseComponent:
    """Base class for all framework components."""

    def __init__(self, config=None):
        self.config = config or settings
        self.initialized = False

    def setup(self):
        """Initialize the component."""
        self.initialized = True

class Application(BaseComponent):
    """Main application class."""

    def __init__(self, settings_module):
        super().__init__(settings_module)
        self.routes = []

    def __call__(self, environ, start_response):
        """WSGI application interface."""
        return self.handle_request(environ, start_response)
