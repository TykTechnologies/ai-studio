# Notification System

Tyk AI Studio includes a centralized Notification System responsible for generating and delivering alerts and messages to users and administrators based on specific system events.

## Purpose

The Notification System aims to:

*   **Inform Stakeholders:** Keep users and administrators aware of important events or required actions.
*   **Enable Proactive Management:** Alert administrators to potential issues or thresholds being reached (e.g., budget limits).
*   **Improve User Experience:** Provide timely feedback on asynchronous processes or user-related events.

## Key Features

*   **Event-Driven:** Notifications are triggered by specific occurrences within the Tyk AI Studio platform.
*   **Configurable Channels:** Supports multiple delivery methods, primarily:
    *   **Email:** Sending notifications to registered user email addresses.
    *   **In-App Notifications:** Displaying messages directly within the Tyk AI Studio UI.
*   **User Preferences:** Allows users (and potentially administrators) to configure which notifications they wish to receive and via which channels (where applicable).
*   **Centralized Logic:** Provides a single system for managing notification templates and delivery rules.

## Common Notification Triggers

Examples of events that might trigger notifications include:

*   **[Budget Control](./llm-management.md#budget-control-system):**
    *   Approaching spending limit threshold (e.g., 80% of budget).
    *   Reaching or exceeding spending limit.
*   **[User Management](./user-management.md):**
    *   New user registration/invitation.
    *   Password reset request.
    *   Changes in user roles or group memberships.
*   **System Health & Errors:**
    *   Significant system errors or failures.
    *   Service degradation alerts.
*   **Security Events:**
    *   Suspicious login activity (if monitored).
    *   Changes to critical security settings.

## Configuration

### SMTP Configuration (Email Notifications)

To enable email notifications, configure the following environment variables:

| Variable | Description | Required |
|----------|-------------|----------|
| `SMTP_SERVER` | SMTP server hostname (e.g., `smtp.gmail.com`) | Yes |
| `SMTP_PORT` | SMTP server port (e.g., `587` for TLS, `465` for SSL) | Yes |
| `SMTP_USER` | SMTP authentication username | Yes |
| `SMTP_PASS` | SMTP authentication password | Yes |
| `FROM_EMAIL` | Sender email address for outgoing notifications | Yes |

**Example `.env` configuration:**

```bash
SMTP_SERVER=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASS=your-app-password
FROM_EMAIL=noreply@your-domain.com
```

> **Note:** If SMTP is not configured, email notifications will be skipped silently, but in-app notifications will still work.

### System-Level Settings (Admin)

Administrators configure core notification settings:
*   SMTP server details (via environment variables above)
*   Default notification templates
*   Enabling/disabling specific system-wide notification types

### User-Level Settings

Users can manage their notification preferences in their profile settings:
*   Opt-in/opt-out of specific notification categories
*   Choose preferred delivery channels (e.g., receive budget alerts via email)

## Integration

The Notification System integrates with various other Tyk AI Studio components that generate relevant events, including:

*   Budget Control System
*   User Management System
*   Analytics System (potentially for performance alerts)
*   Proxy/Gateway (for error or security event alerts)

This system ensures timely communication, helping users and administrators stay informed about the status and activity within the Tyk AI Studio platform.
