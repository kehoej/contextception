"""Admin management tools."""
from webapp.admin.dashboard import DashboardView
from webapp.auth.permissions import check_permission

class ManagementConsole:
    """Admin management console."""

    def __init__(self, user):
        self.user = user
        self.dashboard = DashboardView()

    def has_access(self):
        """Check if user has admin access."""
        return check_permission(self.user, 'admin_access')

    def execute_command(self, command):
        """Execute management command."""
        if not self.has_access():
            return {'error': 'Access denied'}
        # Command execution logic
        return {'success': True}
