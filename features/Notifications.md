# Notifications

## Overview

The notification system in Midsommar centralizes handling of notifications across the platform. Each notification is stored in the database with a unique ID and can be delivered via email or shown within the UI. Notifications serve multiple use cases, including budget alerts, system announcements, and more.

---

## Features

### Notification Storage

- **Database Table**: `notifications` holds each notification, identified by a unique `notification_id` to prevent duplicates.  
- **Metadata**: Each record includes:
  - **Type**: e.g. `"budget_alert"`, `"system_update"`, etc.  
  - **Title**: Short textual summary.  
  - **Content**: Body text with details.  
  - **UserID**: Recipient user.  
  - **Read**: Boolean to track if the user has viewed it.  
  - **SentAt**: Timestamp.  

### Notification Types

- **Budget Alerts**  
  - Sent at 80% and 100% monthly budget usage.  
  - For an app’s budget, both the owner and admins receive them. For an LLM’s budget, only admins do.  
  - Type: `"budget_alert"`.  

- **System Updates**  
  - Maintenance notices or feature announcements.  
  - Type: `"system_update"`.  

- **Other**  
  - Future expansions like security alerts, usage reports, etc.

### Delivery Methods

1. **Email**  
   - Implemented with `MailService` if SMTP is configured (otherwise email-sending is skipped).  
2. **UI**  
   - A bell icon with an unread counter.  
   - A notifications page listing all messages.  
   - Real-time count updates (polled every 60 seconds).  
   - Ability to mark notifications as read/unread.

---

## Notification Model

~~~go
type Notification struct {
    gorm.Model
    NotificationID string // Unique ID for deduplication
    Type           string // e.g. "budget_alert", "system_update", etc.
    Title          string
    Content        string
    UserID         uint
    Read           bool
    SentAt         time.Time
}
~~~

### NotificationService

~~~go
type NotificationService struct {
    db          *gorm.DB
    mailService *MailService
    // Additional fields for concurrency or test storage...
}
~~~

Key methods:

- **`Send(*Notification)`**  
  - Prevents duplicates by checking the `NotificationID`.  
  - Stores to DB; if an SMTP host is set, sends email to the user’s address.  
- **`GetUserNotifications(userID, limit, offset int)`**  
  - Retrieves a paginated list of notifications for the specified user.  
- **`MarkAsRead(notificationID uint)`**  
  - Sets `read = true`.  
- **`GetUnreadCount(userID uint)`**  
  - Returns how many notifications are currently unread.

---

## API Endpoints

| Endpoint                                           | Description                             |
|----------------------------------------------------|-----------------------------------------|
| **GET /common/api/v1/notifications**               | List all notifications for the user     |
| **GET /common/api/v1/notifications/unread/count**  | Get the unread notification count       |
| **PUT /common/api/v1/notifications/:id/read**      | Mark a notification as read             |

**Notes**:
- These endpoints are typically accessed via an authenticated user context.  
- The client polling for unread counts is visible in the admin UI’s NotificationIcon.

---

## UI Components

### 1. **NotificationIcon**  
- Placed in the top navigation bar.  
- Shows unread count (`Badge`).  
- Polls the unread count every 60 seconds.  
- Clicking navigates to the notifications page.

### 2. **NotificationList**  
- Displays all notifications.  
- Unread items highlighted (e.g., different background).  
- “Mark as read” button in each row.

### 3. **NotificationsPage**  
- Dedicated page for the user to review notifications in detail.

### 4. **NotificationContext**  
- A React context that manages unread counts and marking items as read.  
- Updates shared state across the app when notifications change.

---

## Current Usage Locations

1. **Budget Notifications**  
   - **Type**: `"budget_alert"`.  
   - **Triggered**: Whenever an app’s or LLM’s usage crosses 80% or 100% of monthly budget.  
   - **Recipients**:
     - For App budget: the app owner + all admins.  
     - For LLM budget: only admins.  

2. **System Updates**  
   - **Type**: `"system_update"`.  
   - Used for maintenance announcements, feature updates, etc.

3. **App-Specific Notifications**  
   - **Type** can vary, such as `"admin-app-notification"`.  
   - Informs owners about app status changes, configuration updates, or access changes.

### Email Templates

Since email sending is optional (depends on SMTP config), typical templates can be:

1. **budget_alert.tmpl**  
   - For budget threshold notifications (80% or 100%).  
   - Variables: App/LLM name, threshold, current spend, currency.  

2. **admin-notify.tmpl**  
   - For system-wide messages or announcements.  
   - Variables: Title, content.  

3. **admin-app-notification.tmpl**  
   - For app-specific events.  
   - Variables: App name, event details.

---

## Example Usage

**Budget Alert Notification** snippet in `budget_service.go`:

~~~go
notificationID := fmt.Sprintf("budget_app_%d_%d_%d_%d", 
    app.ID, 
    int(spent*100), // convert to cents for dedup 
    int(budget*100),
    threshold,
)

notification := &models.Notification{
    NotificationID: notificationID,
    Type:          "budget_alert",
    Title:         fmt.Sprintf("Budget Alert: App %s at %d%% Usage", app.Name, threshold),
    Content:       content, // from template
    UserID:        userID,  // owner or admin
    Read:          false,
    SentAt:        time.Now(),
}

notificationService.Send(notification)
~~~

---

## Future Enhancements

1. **Additional Notification Types**  
   - Security alerts, usage summations, performance warnings.  
2. **Delivery Methods**  
   - Push notifications (e.g., web push), Slack integration, or webhooks.  
3. **UI Improvements**  
   - Categorizing or filtering notifications (budget vs. system vs. user-level).  
   - Bulk “mark all as read”.  
   - Rich content or HTML formatting.

---

## Conclusion

Midsommar’s **Notification System** provides:

- **Centralized Storage** in the `notifications` table,  
- **Multi-Channel Delivery** (email, in-app UI),  
- **Unique Deduplication** using `notification_id`,  
- **Integration** with Budgeting for alerts,  
- **Real-Time** UI updates using a dedicated poll-based approach.

This ensures critical budget alerts, system updates, and other relevant messages are reliably delivered to the right recipients, bridging cost governance with immediate user awareness.