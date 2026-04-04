"""Authentication views and handlers."""
from webapp.auth.models import User
from webapp.auth.services import AuthService
from webapp.auth.permissions import check_permission

def login_view(request):
    """Handle user login."""
    username = request.get('username')
    password = request.get('password')
    service = AuthService()
    return service.authenticate(username, password)

def logout_view(request):
    """Handle user logout."""
    return {'status': 'logged_out'}

def profile_view(request):
    """Display user profile."""
    if not check_permission(request.user, 'view_profile'):
        return {'error': 'Forbidden'}
    return {'user': request.user}
