"""Email sending utilities."""
from webapp import settings
import smtplib

class EmailSender:
    """Email sender."""

    def __init__(self):
        self.smtp_host = getattr(settings, 'SMTP_HOST', 'localhost')
        self.smtp_port = getattr(settings, 'SMTP_PORT', 587)
        self.from_email = getattr(settings, 'FROM_EMAIL', 'noreply@example.com')

    def send(self, to, subject, body):
        """Send email."""
        # SMTP logic here
        pass

_sender = EmailSender()

def send_email(subject, body, to):
    """Send email using global sender."""
    _sender.send(to, subject, body)

def send_bulk_email(recipients, subject, body):
    """Send email to multiple recipients."""
    for recipient in recipients:
        send_email(subject, body, recipient)
