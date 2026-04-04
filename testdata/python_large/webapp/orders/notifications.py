"""Order notification system."""
from webapp.orders.models import Order
from webapp.utils.email import send_email

def send_order_notification(order):
    """Send order confirmation email."""
    subject = f"Order #{order.id} Confirmation"
    body = f"Your order has been placed. Total: ${order.total}"
    send_email(subject, body, to=order.user_id)

def send_shipping_notification(order, tracking_number):
    """Send shipping notification email."""
    subject = f"Order #{order.id} Shipped"
    body = f"Your order has shipped. Tracking: {tracking_number}"
    send_email(subject, body, to=order.user_id)

def send_delivery_notification(order):
    """Send delivery confirmation."""
    subject = f"Order #{order.id} Delivered"
    body = "Your order has been delivered."
    send_email(subject, body, to=order.user_id)
