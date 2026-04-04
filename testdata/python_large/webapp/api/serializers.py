"""Data serialization for API responses."""
from webapp.core.base import BaseComponent
from webapp.core.types import TypeRegistry

class BaseSerializer(BaseComponent):
    """Base class for serializers."""

    def __init__(self, data=None):
        super().__init__()
        self.data = data
        self.type_registry = TypeRegistry()

    def serialize(self):
        """Convert data to serializable format."""
        return self.data

    def deserialize(self, raw_data):
        """Convert raw data to model instances."""
        return raw_data

class JSONSerializer(BaseSerializer):
    """JSON serialization."""

    def serialize(self):
        """Serialize to JSON-compatible format."""
        import json
        return json.dumps(self.data)
