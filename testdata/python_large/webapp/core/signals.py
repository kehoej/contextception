"""Signal handling for event-driven architecture."""
from webapp.core.base import BaseComponent

class Signal(BaseComponent):
    """Signal dispatcher for event handling."""

    def __init__(self, name):
        super().__init__()
        self.name = name
        self.handlers = []

    def connect(self, handler):
        """Connect a handler to this signal."""
        self.handlers.append(handler)

    def send(self, sender, **kwargs):
        """Send signal to all connected handlers."""
        for handler in self.handlers:
            handler(sender=sender, **kwargs)
