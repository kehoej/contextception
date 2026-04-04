"""Caching utilities."""
from webapp import settings
import time

class Cache:
    """Simple in-memory cache."""

    def __init__(self):
        self.store = {}
        self.ttl = getattr(settings, 'CACHE_TTL', 300)

    def get(self, key):
        """Get cached value."""
        if key in self.store:
            value, expiry = self.store[key]
            if time.time() < expiry:
                return value
            else:
                del self.store[key]
        return None

    def set(self, key, value, ttl=None):
        """Set cached value."""
        ttl = ttl or self.ttl
        expiry = time.time() + ttl
        self.store[key] = (value, expiry)

_cache = Cache()

def cache_get(key):
    """Get from global cache."""
    return _cache.get(key)

def cache_set(key, value, ttl=None):
    """Set in global cache."""
    _cache.set(key, value, ttl)
