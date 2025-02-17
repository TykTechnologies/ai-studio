# Notifications

## Overview
The notification system provides a centralized way to handle notifications across the platform. Each notification is stored in the database and is delivered through multiple channels (email and UI).

## Features

### Notification Storage
- Each notification is stored in the database with a unique ID to prevent duplicates
- Notifications include metadata such as type, title, content, user ID, and read status
- Notifications are timestamped when sent

### Notification Types
- Budget alerts: Sent when budget thresholds (80%, 100%) are reached
- System updates: Used for maintenance notices and feature announcements
- Other notification types can be added in the future

### Delivery Methods
1. Email: Implemented using MailService
2. UI: Fully implemented with:
   - Bell icon with unread counter in top navigation
   - Dedicated notifications page
   - Real-time counter updates
   - Mark as read functionality

### Notification Model
```go
type Notification struct {
    gorm.Model
    NotificationID string // Unique ID to prevent duplicates
    Type           string // e.g. "budget_alert", "system_update", etc.
    Title          string
    Content        string
    UserID         uint
    Read           bool  // For UI display
    SentAt         time.Time
}
```

### Notification Service
The NotificationService provides methods to:
- Send notifications with duplicate prevention
- Mark notifications as read/unread
- Query notifications by user
- Get unread notification count

### API Endpoints
- GET /common/api/v1/notifications - List all notifications for the user
- GET /common/api/v1/notifications/unread/count - Get unread notification count
- PUT /common/api/v1/notifications/:id/read - Mark a notification as read

### UI Components
1. NotificationIcon (ui/admin-frontend/src/admin/components/notifications/NotificationIcon.js)
   - White bell icon in top navigation
   - Shows unread count badge
   - Updates count every minute
   - Navigates to notifications page on click

2. NotificationList (ui/admin-frontend/src/admin/components/notifications/NotificationList.js)
   - Displays all notifications
   - Shows unread notifications with gray background
   - Provides "mark as read" button
   - Marks notifications as read on click

3. NotificationsPage (ui/admin-frontend/src/pages/NotificationsPage.js)
   - Common route accessible to all users
   - Container for NotificationList

4. NotificationContext (ui/admin-frontend/src/admin/context/NotificationContext.js)
   - Manages shared notification state
   - Handles unread count
   - Provides markAsRead functionality
   - Ensures synchronization between components

### Current Usage Locations

1. Budget Notifications
   - Template: templates/budget_alert.tmpl
   - Triggered when:
     * App reaches 80% of budget
     * App reaches 100% of budget
   - Sent to app owner and admins

2. System Updates
   - Template: templates/admin-notify.tmpl
   - Used for:
     * Maintenance announcements
     * Feature updates
     * System alerts

3. App Notifications
   - Template: templates/admin-app-notification.tmpl
   - Used for:
     * App status changes
     * Configuration updates
     * Access changes

### Email Templates
1. budget_alert.tmpl
   - Purpose: Budget threshold notifications
   - Variables: App name, usage percentage, current spend

2. admin-notify.tmpl
   - Purpose: System-wide administrative notifications
   - Variables: Title, content, action required

3. admin-app-notification.tmpl
   - Purpose: App-specific notifications
   - Variables: App name, event type, details

## Example Usage

### Budget Alert Notification
```go
notificationID := fmt.Sprintf("budget_app_%d_%d_%d_%d", 
    app.ID, 
    int(spent*100), // Convert to cents to avoid float in ID
    int(budget*100),
    threshold,
)

notification := &models.Notification{
    NotificationID: notificationID,
    Type:          "budget_alert",
    Title:         fmt.Sprintf("Budget Alert: App %s at %d%% Usage", app.Name, threshold),
    Content:       content,
    UserID:        userID,
    Read:          false,
    SentAt:        time.Now(),
}

notificationService.Send(notification)
```

## Future Enhancements
1. Additional Notification Types
   - Security alerts
   - Usage reports
   - Performance alerts

2. Additional Delivery Methods
   - Push notifications
   - Slack integration
   - Webhook support

3. UI Improvements
   - Notification categories/filters
   - Bulk actions (mark all as read)
   - Notification preferences
   - Rich content support (HTML, markdown)
