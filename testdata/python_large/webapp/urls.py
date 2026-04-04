"""URL routing configuration."""
from webapp.auth import views as auth_views
from webapp.api import routes
from webapp.catalog import views as catalog_views
from webapp.orders import views as order_views
from webapp.admin import dashboard

urlpatterns = [
    ('/auth/', auth_views),
    ('/api/', routes.router),
    ('/catalog/', catalog_views),
    ('/orders/', order_views),
    ('/admin/', dashboard),
]

def get_routes():
    """Return all registered URL patterns."""
    return urlpatterns
