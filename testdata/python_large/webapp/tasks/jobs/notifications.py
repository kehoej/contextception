"""Notification tasks."""
from webapp.tasks.scheduler import Scheduler
from webapp.utils.email import send_email
from webapp.auth.models import User

def send_daily_digest():
    """Send daily digest emails."""
    # Query users
    # Generate digest
    # Send emails
    pass

def send_promotion_emails():
    """Send promotional emails."""
    # Query eligible users
    # Send emails
    pass

def send_reminder_emails():
    """Send reminder emails."""
    users = []  # Query users with reminders
    for user in users:
        send_email('Reminder', 'You have pending items', user.email)

scheduler = Scheduler()
scheduler.schedule(send_daily_digest, interval=86400)
