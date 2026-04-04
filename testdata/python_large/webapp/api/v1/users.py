"""User API endpoints."""
from webapp.auth.models import User
from webapp.auth.services import AuthService
from webapp.api.serializers import BaseSerializer
from webapp.api.errors import APIError

def list_users(request):
    """List all users."""
    serializer = BaseSerializer()
    # Query users
    return {'users': []}

def get_user(request, user_id):
    """Get user by ID."""
    user = User.find_by_username(user_id)
    if not user:
        raise APIError('User not found', 404)
    return BaseSerializer(user).serialize()

def create_user(request):
    """Create new user."""
    service = AuthService()
    user_data = request.json
    # Create user logic
    return {'user': user_data}
