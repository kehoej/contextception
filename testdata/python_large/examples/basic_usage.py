"""Basic usage examples."""
from webapp import settings
from webapp.auth.services import AuthService

def example_authentication():
    """Example: Authenticate a user."""
    service = AuthService()
    result = service.authenticate('username', 'password')
    if result:
        print(f"Authenticated: {result['user'].username}")
        print(f"Token: {result['token']}")

def example_configuration():
    """Example: Access configuration."""
    print(f"Debug mode: {settings.DEBUG}")
    print(f"Database: {settings.DATABASE_URL}")

if __name__ == '__main__':
    example_authentication()
    example_configuration()
