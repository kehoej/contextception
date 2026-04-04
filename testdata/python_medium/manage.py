"""Django-like management script for the shop application."""

from shop.settings import config

import sys


def execute_from_command_line(argv):
    """Run administrative tasks."""
    command = argv[1] if len(argv) > 1 else "help"
    if command == "runserver":
        host = config.get("HOST", "127.0.0.1")
        port = config.get("PORT", 8000)
        print(f"Starting server at {host}:{port}")
    elif command == "migrate":
        print("Running migrations...")
    elif command == "shell":
        print("Opening interactive shell...")
    else:
        print(f"Available commands: runserver, migrate, shell")


if __name__ == "__main__":
    execute_from_command_line(sys.argv)
