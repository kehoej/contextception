"""Type definitions and registry."""
import typing

class TypeRegistry:
    """Registry for custom type definitions."""

    def __init__(self):
        self._types = {}

    def register(self, name: str, type_def: typing.Type):
        """Register a custom type."""
        self._types[name] = type_def

    def get(self, name: str) -> typing.Optional[typing.Type]:
        """Retrieve a registered type."""
        return self._types.get(name)
