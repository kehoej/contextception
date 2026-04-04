"""Advanced configuration examples."""
from webapp import settings
from webapp.core.registry import Registry

def example_custom_settings():
    """Example: Custom settings configuration."""
    # Override default settings
    settings.DEBUG = False
    settings.DATABASE_URL = 'postgresql://localhost/myapp'
    settings.CACHE_TTL = 600

def example_component_registry():
    """Example: Using the component registry."""
    registry = Registry()

    # Register components
    registry.register('cache', object())
    registry.register('logger', object())

    # Retrieve components
    cache = registry.get('cache')
    logger = registry.get('logger')

def example_advanced_config():
    """Example: Advanced configuration patterns."""
    config = {
        'database': settings.DATABASE_URL,
        'debug': settings.DEBUG,
        'secret': settings.SECRET_KEY
    }
    return config

if __name__ == '__main__':
    example_custom_settings()
    example_component_registry()
