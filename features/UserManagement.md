Here is the specification in one nicely formatted Markdown file, enclosed in a single code block using ~~~:

# Midsommar User Management & RBAC Specification

This document provides a comprehensive overview of Midsommar’s **User Management** and **Role-Based Access Control (RBAC)** system. It covers:

1. [Overview of System Capabilities](#overview-of-system-capabilities)  
2. [Authentication Methods & Security Features](#authentication-methods--security-features)  
3. [User Roles & Permissions](#user-roles--permissions)  
4. [Group-Based Access Control](#group-based-access-control)  
5. [Resource Access Management](#resource-access-management)  
6. [Email Notification System](#email-notification-system)  
7. [API Endpoints & Purpose](#api-endpoints--purpose)  
8. [UI Components & Functionality](#ui-components--functionality)  
9. [Frontend Architecture for Permissions & Entitlements](#frontend-architecture-for-permissions--entitlements)  
10. [Security Considerations & Best Practices](#security-considerations--best-practices)  

We also include file references, URLs used in the UI, capabilities, and a discussion of each permission type.

---

## Overview of System Capabilities

Midsommar’s user management system provides:

- **User Registration**: Self-service sign-up with optional domain restrictions (e.g., only users with certain email domains can register).  
- **Email Verification Workflow**: Automatically sends out verification emails upon registration or via “Resend Verification Email” if needed.  
- **Authentication**: Session-based cookie authentication for browser clients, plus API key-based authentication for programmatic usage.  
- **RBAC**: Group-based membership model. Users belong to one or more groups, each providing access to certain resources (catalogues, data catalogues, tool catalogues, etc.).  
- **Admin Role**: First user automatically becomes system admin (with extra capabilities). Admins can manage users, groups, resources, and also receive special notifications.  
- **Password Management**: Password reset, complexity enforcement, and update flows.  
- **Email Notifications**: For system events, new user registrations, password resets, etc.  
- **Customizable UI**: Admin can see “User Management,” “Group Management,” etc., while standard users may have more limited menus.  
- **API Endpoints**: REST endpoints for all major user and group operations, with optional JSON input and pagination.  
- **Security Features**: Bcrypt password hashing, token-based sessions, cookie constraints, domain-based registration restrictions, email verification, etc.  

All capabilities are implemented via a combination of:

- **Backend (Go + Gorm + Gin)**: `auth/`, `services/`, `models/`, `notifications/`, `api/` folders  
- **Frontend (React)**: `/ui/admin-frontend/src/admin/` folder  

---

## Authentication Methods & Security Features

### 1. Session-Based (Cookie) Authentication
- **Primary Mechanism** for browser-based users (especially administrators using the UI).
- **Implementation** in [`auth/auth.go`](../auth/auth.go) & [`api/auth_handlers.go`](../api/auth_handlers.go).  
  - On login, Midsommar sets a session cookie (`session`) with a generated token.  
  - Token expiration is handled by the server (default ~1 hour).  
  - Cookie flags: Secure, HttpOnly, SameSite, Domain, etc., configured in `Config` struct.  
- **Middleware**: `AuthService.AuthMiddleware()` checks for a valid `session_token` from the cookie, query param, or Authorization header.  

### 2. API Key Authentication
- **Purpose**: Programmatic or CLI-based access.  
- **User-Level API Keys**: Each user can have a unique API key stored in `User.APIKey`.  
- **Implementation**:  
  - Generated via `User.GenerateAPIKey()` (cryptographically secure random bytes).  
  - `auth.AuthMiddleware()` looks for `Authorization: Bearer <apiKey>` or `?token=<apiKey>` in the request.  
  - Also used for service-level credentials (e.g., bridging external systems).  
- **Key Rotation**: Via `POST /users/{id}/roll-api-key`, a user can request a new API key in the UI or an admin can do it on their behalf.

### 3. Security & Password Policies
- **Password Hashing**: Bcrypt with a cost factor (`bcrypt.DefaultCost`) to store `User.Password`.  
- **Complexity Enforcement**:  
  - Minimum 8 characters  
  - At least one uppercase, one lowercase, one digit, one special character  
  - Validated in `AuthService.ValidatePasswordComplexity()`.  
- **Reset Tokens**:  
  - `User.ResetToken` and `User.ResetTokenExpiry` (set via `ResetPassword()`).  
  - Expires after configured duration (`config.ResetTokenExpiry`, default 1 hour).  
- **Email Verification**:  
  - `User.VerificationToken` and `User.EmailVerified`.  
  - Triggered on user registration or via `ResendVerificationEmail()`.  
- **Domain Restriction**:  
  - `AllowedRegisterDomains []string` in config ensures only certain email domains can register if not empty.  
- **Cookies**:  
  - Typically `HttpOnly`, `Secure`, `SameSite`, `Domain` can be toggled in `Config`.

### 4. Admin Permissions & Notifications
- **First Registered User**: Automatically is `IsAdmin = true`, `EmailVerified = true`, and `NotificationsEnabled = true`.  
- **Admin**:  
  - Can access advanced endpoints (create groups, delete other users, etc.).  
  - Controlled by `AdminOnly()` middleware.  
- **Notifications**:  
  - If `NotificationsEnabled = true` for an admin, they receive system messages (like new user sign-ups).

---

## User Roles & Permissions

Two principal user “roles”:

1. **Admin**  
   - `User.IsAdmin = true`.  
   - Can manage users, groups, and resources.  
   - Receives new user registration notifications (if `NotificationsEnabled` is also true).  
   - Example: Access to advanced endpoints like `PATCH /groups/{id}`, `DELETE /users/{id}`, etc.  

2. **Standard User**  
   - `User.IsAdmin = false`.  
   - Has limited privileges.  
   - Typically can see only their own resources or those shared via group membership.  
   - Requires email verification before usage.  

Additionally, each user belongs to at least one **Group**, which controls resource permissions.

---

## Group-Based Access Control

Groups are at the heart of the RBAC in Midsommar. They define access to:

- **Catalogues** (standard LLM catalogues)
- **DataCatalogues** (collections of data sources)
- **ToolCatalogues** (collections of tools)
- Possibly **Chats** or other resources

### 1. Groups

**Model**: [`models.Group`](../models/group.go)
```go
type Group struct {
    ID             uint
    Name           string
    Users          []User          // Many-to-many
    Catalogues     []Catalogue     // Many-to-many
    DataCatalogues []DataCatalogue // Many-to-many
    ToolCatalogues []ToolCatalogue // Many-to-many
}
```

- **API endpoints** in `api/group_handlers.go`.  
  - `GET /groups` – List all groups (paginated).  
  - `GET /groups/{id}` – Get details of a group (includes associated resources).  
  - `POST /groups` – Create a group (Admin).  
  - `PATCH /groups/{id}` – Update group name (Admin).  
  - `DELETE /groups/{id}` – Delete group (Admin).  

- **Membership**:  
  - `POST /groups/{id}/users` – Add user to group.  
  - `DELETE /groups/{id}/users/{userId}` – Remove user from group.  

- **Resource Associations**:  
  - `POST /groups/{id}/catalogues` – Add a catalogue to group.  
  - `POST /groups/{id}/data-catalogues` – Add a data catalogue to group.  
  - `POST /groups/{id}/tool-catalogues` – Add a tool catalogue to group.  
  - (And corresponding `DELETE` endpoints.)

### 2. Default Group
- On user registration, if no group named “Default” exists, it is created, and the user is automatically placed in it.  
- Ensures each user has a baseline membership.

### 3. Resource Access
- A user inherits access to all resources attached to their groups.  
- E.g., if Group #1 has a “Data Catalogue #5,” then all members of Group #1 can see or use that data catalogue.

---

## Resource Access Management

1. **Catalogues**  
   - Found in `[models.Catalogue]`, typically representing a set of LLMs or AI endpoints.  
   - A group can hold multiple catalogues, so any user in that group can interact with them.

2. **Data Catalogues**  
   - Found in `[models.DataCatalogue]`, each describing curated data sources.  
   - By adding the data catalogue to a group, members can query the data sources in that catalogue.

3. **Tool Catalogues**  
   - Found in `[models.ToolCatalogue]`, representing sets of integrated tools or functionalities.  
   - Only group members can access these tools from the UI or the API.

4. **Entitlements**  
   - The system merges all resources from all groups a user belongs to and exposes them as “entitlements.”  
   - **Implementation**: See `GetUserEntitlements(userID)` in `services/user_service.go`.  
   - Aggregates catalogues, data catalogues, tool catalogues, and associated chats from each group, removing duplicates.  
   - Used by the frontend to show/hide certain functionalities in the UI.

---

## Email Notification System

### 1. Overview
- Facilitated by `MailService` in `notifications/email.go`.  
- Emails are only sent if SMTP is configured.  
- Plain text templates in `/templates`.

### 2. Key Email Types
1. **Password Reset**  
   - Template: `templates/reset.tmpl`.  
   - Called by `AuthService.ResetPassword()`.  

2. **Admin Notification**  
   - Template: `templates/admin-notify.tmpl`.  
   - Sent to all admin users with `NotificationsEnabled = true` (and the `adminEmail` from config if not in that set).  
   - Example usage: `AuthService.notifyAdmin(user)` upon user registration.

3. **Verification Email**  
   - Sent after registration or via “Resend Verification.”  
   - Contains a link `GET /auth/verify-email?token=...`.  
   - Once user clicks, `AuthService.VerifyEmail()` sets `EmailVerified = true`.

### 3. NotificationService
- In `services/notification_service.go`.  
- Creates, stores, and de-duplicates notifications (some are also purely email-based).  
- Admin notifications or user-level messages.

---

## API Endpoints & Purpose

Below is a non-exhaustive summary of critical endpoints. For full details, see the code in:

- `api/auth_handlers.go`
- `api/user_handlers.go`
- `api/group_handlers.go`

| **Endpoint**                         | **Method** | **Description**                                                   | **Secured?**        |
|-------------------------------------|-----------|-------------------------------------------------------------------|---------------------|
| **Auth**                             |           |                                                                   |                     |
| /auth/login                          | POST      | User login. Sets a session cookie if valid credentials.           | No                  |
| /auth/logout                         | POST      | Logout current user. Clears session cookie.                       | Yes                 |
| /auth/register                       | POST      | Register new user. Potentially restricted by domain.              | No                  |
| /auth/verify-email                   | GET       | Verify user email (uses token param).                             | No                  |
| /auth/forgot-password                | POST      | Initiate password reset flow.                                     | No                  |
| /auth/reset-password                 | POST      | Complete password reset with token.                               | No                  |
| /auth/resend-verification            | POST      | Resend verification email.                                        | No                  |
| **User**                             |           |                                                                   |                     |
| /users                               | GET       | List users (paginated).                                           | Yes (Admin)         |
| /users                               | POST      | Create new user (Admin).                                          | Yes (Admin)         |
| /users/{id}                          | GET       | Get user by ID.                                                   | Yes                 |
| /users/{id}                          | PATCH     | Update user.                                                      | Yes                 |
| /users/{id}                          | DELETE    | Delete user.                                                      | Yes (Admin)         |
| /users/{id}/roll-api-key             | POST      | Regenerate user’s API key.                                        | Yes                 |
| /users/{id}/catalogues               | GET       | List all catalogues user can access.                              | Yes                 |
| /users/{id}/groups                   | GET       | List all groups the user belongs to.                              | Yes                 |
| **Groups**                           |           |                                                                   |                     |
| /groups                              | GET       | List all groups (paginated).                                      | Yes                 |
| /groups                              | POST      | Create new group.                                                 | Yes (Admin)         |
| /groups/{id}                         | GET       | Get group details.                                                | Yes                 |
| /groups/{id}                         | PATCH     | Update group name.                                                | Yes (Admin)         |
| /groups/{id}                         | DELETE    | Delete group.                                                     | Yes (Admin)         |
| /groups/{id}/users                   | GET       | List users in a group.                                            | Yes                 |
| /groups/{id}/users                   | POST      | Add user to group.                                                | Yes (Admin)         |
| /groups/{id}/users/{userId}          | DELETE    | Remove user from group.                                           | Yes (Admin)         |
| /groups/{id}/catalogues             | GET       | List group’s catalogues.                                          | Yes                 |
| /groups/{id}/catalogues             | POST      | Add a catalogue to group.                                         | Yes (Admin)         |
| /groups/{id}/catalogues/{catalogueId}| DELETE    | Remove a catalogue from group.                                    | Yes (Admin)         |
| /groups/{id}/data-catalogues        | GET       | List data catalogues in a group.                                  | Yes                 |
| /groups/{id}/data-catalogues        | POST      | Add a data catalogue to group.                                    | Yes (Admin)         |
| /groups/{id}/data-catalogues/{id}   | DELETE    | Remove data catalogue from group.                                 | Yes (Admin)         |
| /groups/{id}/tool-catalogues        | GET       | List tool catalogues in a group.                                  | Yes                 |
| /groups/{id}/tool-catalogues        | POST      | Add a tool catalogue to group.                                    | Yes (Admin)         |
| /groups/{id}/tool-catalogues/{id}   | DELETE    | Remove tool catalogue from group.                                 | Yes (Admin)         |

Security: The system uses `AuthMiddleware()` and `AdminOnly()` for route protection. Non-Admin routes typically require a valid user session or API key.

---

## UI Components & Functionality

The admin UI is located in:  
`/ui/admin-frontend/src/admin/components/` and `/ui/admin-frontend/src/admin/pages/`.

### 1. Users

- **Paths**:
  - `/admin/users`: Lists users (see `pages/Users.js`)
  - `/admin/users/new`: Create new user
  - `/admin/users/edit/:id`: Edit existing user
  - `/admin/users/:id`: View user details

- **Implementation**:
  - `UserForm.js` – Form to add/edit user details (email, name, admin status, password, etc.).
  - `UserDetails.js` – Detailed view for user info, chat history, API key management, etc.

- **Key Capabilities**:
  - Create/Edit: Name, Email, Password, Admin toggle, ShowPortal/ShowChat toggles, Email Verified toggle, Notifications enabled toggle (only for Admin).
  - **Group Membership**: Users can be assigned or removed from groups.
  - **API Key**: Rotated via a button that calls `POST /users/{id}/roll-api-key`.

### 2. Groups

- **Paths**:
  - `/admin/groups`: Lists groups (see `pages/Groups.js`)
  - `/admin/groups/new`: Create a new group
  - `/admin/groups/edit/:id`: Edit existing group
  - `/admin/groups/:id`: View group detail

- **Implementation**:
  - `GroupForm.js` – Form to create/update group name.
  - `GroupDetail.js` – Detailed view showing group’s users and assigned resources.

- **Key Capabilities**:
  - **Group CRUD**: Create, rename, delete groups.
  - **Membership**: Add/Remove users from the group.
  - **Resource Assignment**: Add/Remove catalogues, data catalogues, tool catalogues to/from the group.

### 3. Entitlements / Resource Access

- The front end uses the entitlements logic from the `/common/me` endpoint to filter displayed features.
- Entitlements are cached in localStorage (example code in `hooks/useUserEntitlements.js`).

### 4. Navigation & Layout

- The Admin UI uses Material-UI and custom styling from `sharedStyles.js`.
- Top Nav includes quick links like “Users,” “Groups,” etc., for admin usage.
- **Authentication**:
  - On `401 Unauthorized`, the UI automatically redirects to `/login`.

### 5. URLs & Routes

- **Login**: `/login` – Uses the public “auth” route.
- **Admin**: All admin features start with `/admin/...`.  
  - `/admin/users`  
  - `/admin/groups`  
  - Additional sub-routes for details, editing, creation.

---

## Frontend Architecture for Permissions & Entitlements

### 1. Checking User Permissions

- **Admin check**: The UI reads `user.attributes.is_admin`. If true, user sees advanced actions.
- For group-based resource checks, the UI merges the user’s entitlements:
  - `catalogues`
  - `data_catalogues`
  - `tool_catalogues`
  - Additional flags like `show_chat`, `show_portal`.

- The React code calls `/api/v1/me` (or an equivalent) to retrieve `UserWithEntitlementsResponse`.

### 2. Entitlements & Caching

- Defined in `services/user_service.go: GetUserEntitlements()`.
- The React hook `useUserEntitlements` (found in `hooks/useUserEntitlements.js`) fetches `/common/me`:
  - Caches data in local storage for ~10 seconds.
  - Minimizes repeated calls.
- The UI then conditionally renders features if the user is an admin or if the resource is in the user’s entitlements list.

### 3. Example: Checking if a User can Access a “Data Source”

- The backend merges the user’s data sources from each group’s data catalogue.
- The UI or the proxy verifies user membership prior to usage.

---

## Security Considerations & Best Practices

1. **Password Complexity**: Bcrypt hashing plus mandatory uppercase, lowercase, digit, special character.  
2. **Session Cookie**:  
   - Mark as `HttpOnly` to prevent JavaScript reads.  
   - Use `Secure` for HTTPS.  
   - `SameSite` set to Strict or Lax to mitigate CSRF.  
3. **API Key**:  
   - Treat as secret.  
   - Rotatable on demand.  
   - Store in user’s profile.  
   - Minimum 32 bytes random for cryptographic strength.  
4. **Registration Restrictions**:  
   - Admin can disable open registrations (`RegistrationAllowed = false`).  
   - Or restrict by domain (`AllowedRegisterDomains`).  
5. **Email Verification**:  
   - Prevents malicious sign-ups or spam.  
   - Required for login unless it is the first user (admin).  
6. **Privilege Separation**:  
   - Admin endpoints require `IsAdmin = true`.  
   - Non-admin endpoints require only standard user session or API key.  
7. **Auditing**:  
   - Potential extension for logging who created/deleted users or groups.  
   - Not fully implemented by default.  
8. **Notifications**:  
   - Admin can enable or disable notifications.  
   - System sends new user registration notices to all admin users with notifications enabled.  
9. **GORM Entities**:  
   - All user data is stored in `users` table.  
   - Group memberships in `user_groups` many-to-many table.  
   - For resource references (catalogues, data catalogues, etc.), also many-to-many associations.

---

## Conclusion

Midsommar’s User Management & RBAC system combines:

- **Secure Authentication** (session cookies + API keys).  
- **Robust Group-Based Access Control** to resources (catalogues, data, tools).  
- **Flexible Registration** with domain restrictions and mandatory email verification.  
- **Admin Tools** for user/group lifecycle management.  
- **Integrated Notification System** for password reset, new user, and admin announcements.  
- **Modular Frontend** that uses entitlements to show/hide features.

By tying it all together, the system ensures secure, scalable management of users, roles, and resources, with a user-friendly admin UI and robust programmatic endpoints.

If you need more diagrams, additional code references, or other clarifications, please let us know, and we can extend this document.