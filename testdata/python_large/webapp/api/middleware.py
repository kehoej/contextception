"""API middleware."""
from webapp.auth.middleware import AuthMiddleware
from webapp.auth.tokens import verify_token

class APIAuthMiddleware(AuthMiddleware):
    """API-specific authentication middleware."""

    def __call__(self, request):
        """Authenticate API requests."""
        token = request.headers.get('X-API-Token') or request.headers.get('Authorization')
        if not token:
            return {'error': 'Authentication required'}

        user_data = verify_token(token)
        if not user_data:
            return {'error': 'Invalid token'}

        return super().__call__(request)

api_auth = APIAuthMiddleware(None)
