# Notifications

## Introduction

This document explains Midsommar's **Notification System** in depth, including how it integrates with the backend, database models, services, and frontend UI. Additionally, it proposes enhancements to support more robust notification features and future expansions.

---

## Table of Contents

1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Key Components](#key-components)
4. [Notification Flow](#notification-flow)
5. [Notification Types](#notification-types)
6. [Delivery Methods](#delivery-methods)
7. [Permissions and Settings](#permissions-and-settings)
8. [Code References](#code-references)
9. [Testing Strategy](#testing-strategy)
10. [UI Integration](#ui-integration)
11. [Suggested Improvements](#suggested-improvements)
12. [Future Enhancements](#future-enhancements)
13. [Conclusion](#conclusion)

---

## Overview

The **Midsommar Notification System** centralizes the management and delivery of notifications across the platform. Its core objectives are:

- **Consistent & Uniform Delivery:** Provide a unified approach to sending in-app and email notifications.
- **Deduplication & Tracking:** Use a unique `notification_id` for each record to avoid sending duplicates, and store read/unread status.
- **Multi-Channel Delivery:** Deliver notifications via:
  - Email (using the configured SMTP server).
  - In-App UI with real-time checks for unread messages.
- **Extensible & Configurable:** Additional notification types or channels can be added with minimal overhead.

---

## System Architecture

1. **Database Table:** A `notifications` table holds each notification ([models/notifications.go](../models/notifications.go)).
2. **NotificationService:** A dedicated service layer ([services/notification_service.go](../services/notification_service.go)) orchestrates creating, storing, deduplicating, and sending notifications.
3. **Email Integration:** The `MailService` handles email dispatch if an SMTP host is configured ([notifications/email.go](../notifications/email.go)).
4. **Budget Integration:** A portion of the system automatically sends **budget alerts** (80% or 100% usage) via the `BudgetService` ([services/budget_service.go](../services/budget_service.go)), illustrating how domain-specific logic can fire notifications.
5. **API Endpoints:** REST endpoints under `/common/api/v1/notifications` provide listing, unread count, and "mark as read" functionality ([api/notification_handlers.go](../api/notification_handlers.go)).
6. **Auth Integration:** The `auth.go` logic can trigger admin notifications about new user registrations or changes ([auth.go](../auth/auth.go)).
7. **Proxy Integration:** The [proxy/analyze_utils.go](../proxy/analyze_utils.go) triggers budget usage analysis, which may generate notifications if budgets exceed thresholds.

---

## Key Components

### 1. Notification Model

**File Location:** [models/notifications.go](../models/notifications.go)

~~~go
type Notification struct {
    gorm.Model
    NotificationID string `gorm:"uniqueIndex"` // Unique for deduplication
    Type           string // e.g. "budget_alert", "system_update", "admin_app_notification", etc.
    Title          string
    Content        string
    UserID         uint
    Read           bool      // Track if viewed
    SentAt         time.Time // When the notification was sent
}
~~~

### 2. User Model with Notification Settings

**File Location:** [models/user.go](../models/user.go)

```go
type User struct {
    // ... standard user fields ...
    IsAdmin              bool
    NotificationsEnabled bool // If true, receives admin-level notifications
}
```

### 3. NotificationService

**File Location:** [services/notification_service.go](../services/notification_service.go)

```go
type NotificationService struct {
    db          *gorm.DB
    mailService *MailService
    // ...
}
```

- **Send(*Notification)**
  - Deduplicates via `NotificationID`
  - Persists to the database
  - Dispatches an email if SMTP is configured
- **SendAdminAppNotification(title, content string)**
  - Sends notifications to all admin users with `NotificationsEnabled = true`
  - Also sends an email to `config.Get().AdminEmail` if it differs from the admins' addresses
- **GetUserNotifications(userID, limit, offset int)**
  - Returns a paginated list of notifications for the specified user.
- **GetUnreadCount(userID uint)**
  - Returns the count of unread notifications for the user.
- **MarkAsRead(notificationID uint)**
  - Updates a notification’s status to read in the database.

### 4. MailService & Email Sending

**File Location:** [notifications/email.go](../notifications/email.go)

- Handles email sending if `SMTPHost != ""`
- Uses the `go-mail/mail` library for constructing and dispatching emails.
- **TestMailer** (see [test_mailer.go](../notifications/test_mailer.go)) captures outbound emails for testing purposes.

### 5. BudgetService Integration

**File Location:** [services/budget_service.go](../services/budget_service.go)

- Leverages the NotificationService to send “budget_alert” notifications at 80% or 100% usage.
- Differentiates between:
  - App budget alerts (sent to the app owner and all admins)
  - LLM budget alerts (sent only to admins)
- Uses a composite `notification_id` incorporating the entity, threshold, and budget period for uniqueness.

### 6. API Endpoints

**File Location:** [api/notification_handlers.go](../api/notification_handlers.go)

| Endpoint                                           | Description                                                           |
| -------------------------------------------------- | --------------------------------------------------------------------- |
| GET `/common/api/v1/notifications`                 | Lists notifications for the user                                      |
| GET `/common/api/v1/notifications/unread/count`    | Returns the unread notification count                                 |
| PUT `/common/api/v1/notifications/:id/read`        | Marks a specific notification as read                                 |

> These endpoints are protected by user authentication (via `auth.AuthMiddleware()` in [api/api.go](../api/api.go)), ensuring that users only access their own notifications.

---

## Notification Flow

1. **Trigger:** A domain-specific event occurs (e.g., budget threshold exceeded, new user registration, new app creation).
2. **Service Call:** The corresponding service (e.g., BudgetService, AuthService, or custom logic) invokes `NotificationService.Send(...)`.
3. **Deduplication & DB Storage:** The system checks if the `notification_id` has been used; if not, it persists the notification.
4. **Email Dispatch:** If `mailService` is configured (i.e., a valid SMTP setup exists), an email is sent.
5. **Frontend Poll/Fetch:** The UI periodically calls the `/unread/count` endpoint (default every 60 seconds) and fetches the notifications list via `/notifications`.
6. **User Reads Notification:** Upon viewing, the backend updates the notification's `Read` status to `true`.

---

## Notification Types

Midsommar currently supports:

1. **Budget Alerts ("budget_alert")**
   - Triggered by BudgetService at 80% or 100% usage.
   - Sent to the app owner and admins (for apps) or only admins (for LLM budgets).

2. **System Updates ("system_update")**
   - Used for scheduled maintenance or feature announcements.
   - Can be broadcast to all users or targeted groups based on domain logic.

3. **Admin App Notifications ("admin_app_notification")**
   - For events like new app creation, credential approval, or usage anomalies.
   - Delivered to admin users with `NotificationsEnabled = true` and optionally to `config.Get().AdminEmail`.

4. **(Proposed) User Registration Notifications**
   - Currently integrated in `auth.go` for alerting admins about new signups.
   - Can be refined as a distinct type, e.g. `"user_signup"`.

The `Type` field in the Notification model allows for easy expansion of notification types as new domain events emerge.

---

## Delivery Methods

1. **Email**
   - If `MailService.SMTPHost != ""`, each call to `Send()` attempts to dispatch an email.
   - Templated messages are located in the `templates/` directory (e.g., `budget_alert.tmpl`, `admin-notify.tmpl`).

2. **In-App UI**
   - **NotificationIcon** ([src/admin/components/notifications/NotificationIcon.js](../src/admin/components/notifications/NotificationIcon.js)) displays a badge with the unread count.
   - **NotificationList** ([NotificationList.js](../src/admin/components/notifications/NotificationList.js)) shows all notifications.
   - **NotificationContext** ([NotificationContext.js](../src/admin/components/notifications/NotificationContext.js)) manages fetching, state sharing, and marking notifications as read.
   - **NotificationsPage** ([NotificationsPage.js](../src/admin/pages/NotificationsPage.js)) offers a dedicated view for notifications.

---

## Permissions and Settings

1. **Admin vs Non-Admin**
   - **Admins:** Can enable `NotificationsEnabled` and receive additional system-wide notifications.
   - **Non-admins:** Receive direct notifications (e.g., app-specific budget alerts).

2. **NotificationsEnabled**
   - A boolean flag in the User model ([models/user.go](../models/user.go)).
   - When disabled, admin-level notifications are not sent to that user.
   - By default, the super admin (first user) is set with `NotificationsEnabled = true`.

3. **Email Verification**
   - Email dispatch does not strictly require verified emails, though the system tracks `EmailVerified`.
   - Additional checks can be incorporated within `NotificationService.Send()` or `AuthService` to enforce verification.

---

## Code References

Below is a map of files contributing to the Notification System:

- **Email Functionality:**
  - [notifications/email.go](../notifications/email.go): Core email functionality, including `MailService.SendEmail()`.
  - [notifications/test_mailer.go](../notifications/test_mailer.go) & associated tests: Capturing test emails to verify email logic.
- **Data Models:**
  - [models/notifications.go](../models/notifications.go): Notification model definition.
  - [models/user.go](../models/user.go): User model including `NotificationsEnabled`.
- **Services:**
  - [services/notification_service.go](../services/notification_service.go): Manages creation, sending, and storage of notifications.
    - *Tests:* `notification_service_test.go`.
  - [services/budget_service.go](../services/budget_service.go): Connects budget usage events to notifications.
    - *Tests:* `budget_service_test.go`.
- **API Endpoints:**
  - [api/notification_handlers.go](../api/notification_handlers.go) & [api/api.go](../api/api.go): Define REST endpoints and integrate them with the main router.
- **Authentication:**
  - [auth/auth.go](../auth/auth.go): Triggers admin notifications on new user registrations.
- **Proxy Analysis:**
  - [proxy/analyze_utils.go](../proxy/analyze_utils.go): Generates notifications based on budget usage analysis.
- **Frontend Components:**
  - `NotificationList.js`
  - `NotificationIcon.js`
  - `NotificationContext.js`
  - `NotificationsPage.js`
  - `UserForm.js` (for admin user creation/edit, toggles `NotificationsEnabled`)
  - `UserDetails.js` (displays the notifications setting)

---

## Testing Strategy

1. **Unit Tests**
   - `notification_service_test.go`: Validates deduplication, database persistence, and email dispatch using mock mailers.
   - `test_mailer.go`: Ensures email logic works without sending real emails.

2. **Integration Tests**
   - **Budget Alerts:** Use tests (e.g., `proxy_budget_test.go`) to simulate real usage events and validate notification creation.
   - **Auth Registration:** Confirm that new user signups trigger the appropriate admin notifications through the service chain (AuthService → NotificationService).

3. **UI Tests**
   - Components like `NotificationList` and `NotificationIcon` can be tested with tools such as React Testing Library or Cypress to verify:
     - Correct fetching and rendering of notifications.
     - Proper updating of the unread counter.

4. **Recommended Enhancements**
   - Increase test coverage for advanced scenarios (e.g., environments with SMTP disabled, concurrency handling, stress testing with a high volume of notifications).
   - Introduce snapshot tests for email templates to ensure message consistency and correctness of placeholders.

---

## UI Integration

### Notification Menu & Polling

- **NotificationIcon.js**
  - Renders a bell icon with an unread count badge.
  - Polls every 60 seconds via `fetchUnreadCount()` from `/common/api/v1/notifications/unread/count`.
  - On click, navigates to the notifications listing page (`/notifications`).

### Notifications Page

- **NotificationsPage.js**
  - Hosts the `NotificationList.js` component.
- **NotificationList.js**
  - Retrieves notifications from `/common/api/v1/notifications`.
  - Provides functionality to mark individual notifications as read (via PUT to `/common/api/v1/notifications/:id/read`).
  - Distinguishes unread items visually.

### URLs & Routes

- **Frontend Route:** `/notifications` renders the NotificationsPage.
- **Backend Endpoint:** `/common/api/v1/notifications` handles list, read, and unread count operations.

### Permissions & Visibility

- **Non-admin users:** Do not see the admin toggle for notifications but receive user-specific alerts (e.g., budget alerts for their apps).
- **Admin users:** Can toggle `NotificationsEnabled` within `UserForm.js` and view it in `UserDetails.js`.

---

## Suggested Improvements

While robust, the notification system can benefit from further enhancements:

1. **Notification Categories & Filters**
   - Introduce a category field (or expand the `Type` field) to enable filtering (e.g., “budget”, “security”, “system update”).
   - Enhance the NotificationsPage with filtering options.

2. **Global or Team-Level Notifications**
   - Allow broadcast notifications to multiple teams or groups rather than only individual records.
   - Consider implementing a `group_id` reference or a multi-user broadcast model.

3. **Optional Real-Time Updates**
   - Replace the 60-second polling with WebSocket or Server-Sent Events (SSE) for instant UI updates.

4. **"Mark All As Read"**
   - Add an endpoint or UI control to mark all notifications as read in one action.

5. **Delivery Preferences**
   - Split the `NotificationsEnabled` setting into separate toggles for in-app and email notifications, giving admins finer control.

6. **Multi-Language Support**
   - Externalize message strings or utilize templates that support localization based on user preferences.

7. **Scalability & Performance**
   - For high volumes, consider offloading notifications to a dedicated queue or microservice.
   - Introduce asynchronous processing for complex notifications.

---

## Future Enhancements

1. **Security Alerts**
   - Implement alerts for suspicious login attempts, changes to sensitive settings, or 2FA reminders.

2. **Usage Summaries**
   - Send weekly or monthly usage summaries to users or app owners.

3. **Task & Collaboration Notifications**
   - If Midsommar expands into collaboration or task management, develop a central “task update” notification type.

4. **Customizable Notification Rules**
   - Allow advanced users to define custom triggers (e.g., “notify me if usage exceeds $X/day for my app” or “alert on new user signups under my domain”).

5. **Slack / Chat Integrations**
   - Expand beyond email by integrating with Slack, MS Teams, or other chat platforms via webhooks for immediate notifications.

---

## Conclusion

Midsommar’s Notification System is a core component that provides:

- Centralized management of various notification types.
- Deduplication and persistent storage in the notifications table.
- Multi-channel delivery (in-app and email).
- Configurability for both admins and end users via the `NotificationsEnabled` setting.
- Seamless integration with budgeting, user authentication, and other domain-specific processes.

To evolve further, the system should:
1. Introduce specialized categories or filters.
2. Enhance user customization options.
3. Explore real-time and multi-channel integrations.
4. Strengthen testing to cover broader and more complex scenarios.
