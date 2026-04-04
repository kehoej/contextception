"""Base classes for models and views."""

from mylib.config import Config


class BaseModel:
    """Abstract base for all models."""

    def __init__(self):
        self.config = Config()

    def validate(self):
        raise NotImplementedError

    def to_dict(self):
        raise NotImplementedError


class BaseView:
    """Abstract base for views."""

    def __init__(self, config):
        self.config = config

    def render(self, template, **context):
        return template.format(**context)
