"""Data formatting utilities."""
import json
import re

def format_json(data, indent=2):
    """Format data as pretty JSON."""
    return json.dumps(data, indent=indent)

def format_currency(amount):
    """Format amount as currency string."""
    return f"${amount:.2f}"

def slugify(text):
    """Convert text to URL-safe slug."""
    text = text.lower()
    text = re.sub(r'[^\w\s-]', '', text)
    text = re.sub(r'[\s_-]+', '-', text)
    return text.strip('-')
