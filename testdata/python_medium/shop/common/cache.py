"""Simple in-memory caching utilities."""

from shop.settings import config
from shop.common.utils import generate_id

import time


_cache = {}


def cache_get(key):
    """Retrieve a value from the cache if it exists and hasn't expired."""
    entry = _cache.get(key)
    if entry is None:
        return None
    if time.time() > entry["expires_at"]:
        del _cache[key]
        return None
    return entry["value"]


def cache_set(key, value, ttl=None):
    """Store a value in the cache with an optional TTL."""
    if ttl is None:
        ttl = config.get("CACHE_TTL", 300)
    _cache[key] = {
        "id": generate_id(),
        "value": value,
        "expires_at": time.time() + ttl,
    }


def cache_delete(key):
    """Remove an entry from the cache."""
    _cache.pop(key, None)


def cache_clear():
    """Remove all entries from the cache."""
    _cache.clear()


def cache_stats():
    """Return basic cache statistics."""
    now = time.time()
    active = sum(1 for e in _cache.values() if e["expires_at"] > now)
    return {"total": len(_cache), "active": active, "expired": len(_cache) - active}
