# Single Sign-On (SSO) Support

## Introduction

This document provides a comprehensive overview of Midsommar's **Single Sign-On (SSO)** support system, detailing its architecture, components, and integration points across the platform. The SSO system enables authentication with external identity providers while maintaining security and access controls through the embedded Tyk Identity Broker (TIB).

---

## Table of Contents

1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Key Components](#key-components)
4. [Tyk Identity Broker Integration](#tyk-identity-broker-integration)
5. [Authentication Flow](#authentication-flow)
6. [Supported Providers](#supported-providers)
7. [User Group Mapping](#user-group-mapping)
8. [API Endpoints](#api-endpoints)
9. [UI Components](#ui-components)
10. [Security Considerations](#security-considerations)
11. [Configuration Requirements](#configuration-requirements)
12. [Future Enhancements](#future-enhancements)

---

## Overview

The **Midsommar SSO System** provides a framework for integrating external identity providers with the platform's authentication system. Its core objectives are:
- **Multiple Provider Support:** Enable authentication via OpenID Connect, SAML, LDAP, and Social providers
- **Profile Management:** Create, configure, and manage SSO profiles through a dedicated Monaco editor-based UI
- **Group Mapping:** Map external identity provider groups to internal Midsommar groups with support for multiple group assignments
- **Secure Authentication:** Enforce security best practices for authentication flows
- **Admin Controls:** Restrict SSO configuration to administrators
- **Seamless Integration:** Embedded Tyk Identity Broker eliminates the need for external identity management services
- **Default Login Profile:** Configure a default SSO profile to appear directly on the login page for streamlined authentication
- **Seamless Integration:** Embedded Tyk Identity Broker eliminates the need for external identity management services

---

## System Architecture

1. **Database Models:**
   - `Profile` model ([models/tib_profiles.go](../models/tib_profiles.go)) - Core SSO profile configuration including provider settings, identity handler configuration, and group mappings
   - `GormAuthRegisterBackend` model ([models/tib_backend_store.go](../models/tib_backend_store.go)) - GORM implementation of TIB's AuthRegisterBackend interface for profile storage and retrieval
   - `GormKVStore` model ([models/tib_kv_store.go](../models/tib_kv_store.go)) - Key-value store implementation with dual usage: (1) used internally by the embedded TIB as its configuration handler, and (2) used by the SSO service to store NonceData and authentication tokens, utilizing the `KVPair` struct for database persistence
   - `User` model ([models/user.go](../models/user.go)) - Extended to support SSO authentication and group memberships
   - `StringMap` and `JSONMap` types - Custom types for storing profile configuration as JSON in the database

2. **Services:**
   - `Service` ([services/profile_service.go](../services/profile_service.go)) - Handles SSO profile CRUD operations with validation
   - `SSOService` ([services/sso_service.go](../services/sso_service.go)) - Core service that manages SSO authentication flows with the following features:
     - Embeds and initializes Tyk Identity Broker for provider integration
     - Implements nonce token generation and validation using the KV store
     - Manages user creation and group assignment during authentication
     - Integrates with the notification service for user creation events

3. **API Layer:**
   - Profile management endpoints ([api/profile_handlers.go](../api/profile_handlers.go)) - CRUD operations for SSO profiles
   - Authentication endpoints ([api/auth_handlers.go](../api/auth_handlers.go)) - Handles SSO authentication flows
   - Middleware for authentication and authorization checks

4. **Frontend Components:**
   - SSO profile management UI ([ui/admin-frontend/src/admin/pages/SSOProfiles.js](../ui/admin-frontend/src/admin/pages/SSOProfiles.js)) - List and manage SSO profiles
   - SSO profile editor ([ui/admin-frontend/src/admin/components/sso-profiles/SSOProfileEditor.js](../ui/admin-frontend/src/admin/components/sso-profiles/SSOProfileEditor.js)) - Monaco-based editor for profile configuration
   - SSO profile details ([ui/admin-frontend/src/admin/components/sso-profiles/SSOProfileDetails.js](../ui/admin-frontend/src/admin/components/sso-profiles/SSOProfileDetails.js)) - Detailed view of SSO profile configuration
   - Provider configuration components in the provider-config directory - Provider-specific field sets for different authentication types
   
   Note: All SSO routes are conditionally rendered based on the user having admin privileges with the `AccessToSSOConfig` permission. Routes are only accessible when `uiOptions.ShowSSOConfig` is true.

---

## Key Components

### 1. Profile Model

**File Location:** [models/tib_profiles.go](../models/tib_profiles.go)

The Profile model stores all configuration for an SSO integration:

```go
type Profile struct {
    gorm.Model                `json:"-"`
    ProfileID                 string `gorm:"index" json:"ID"`
    Name                      string
    OrgID                     string
    ActionType                string
    MatchedPolicyID           string
    Type                      string
    ProviderName              string
    CustomEmailField          string
    CustomUserIDField         string
    ProviderConfig            JSONMap `gorm:"type:json"`
    IdentityHandlerConfig     JSONMap `gorm:"type:json"`
    ProviderConstraintsDomain string
    ProviderConstraintsGroup  string
    ReturnURL                 string
    DefaultUserGroupID        string
    CustomUserGroupField      string
    UserGroupMapping          StringMap `gorm:"type:json"`
    UserGroupSeparator        string
    SSOOnlyForRegisteredUsers bool
    SelectedProviderType      string `json:"-"`
    UserID                    uint   `json:"-"`
}
```

The model includes methods for CRUD operations and conversion to TIB's tap.Profile format:
- `Create` - Persists a new profile to the database
- `Get` - Retrieves a profile by ID
- `Update` - Updates an existing profile
- `Delete` - Removes a profile
- `MapToTapProfile` - Converts the model to TIB's format for authentication

### 2. SSO Handlers

**File Location:** [api/sso_handlers.go](../api/sso_handlers.go)

The SSO handlers process authentication requests and manage the SSO flow:

- `handleTIBAuth` - Initiates authentication with the identity provider
- `handleTIBAuthCallback` - Processes callbacks from the identity provider
- `handleSAMLMetadata` - Serves SAML metadata for identity providers
- `handleSSO` - Processes authentication with nonce tokens
- `handleNonceRequest` - Creates nonce tokens for SSO authentication

### 3. Profile Handlers

**File Location:** [api/profile_handlers.go](../api/profile_handlers.go)

The profile handlers provide the API endpoints for managing SSO profiles:

- `createProfile` - Creates a new SSO profile
- `getProfile` - Retrieves a profile by ID
- `updateProfile` - Updates an existing profile
- `deleteProfile` - Deletes a profile
- `listProfiles` - Lists all profiles with pagination
- Handles profile serialization and deserialization for API responses

---

## Tyk Identity Broker Integration

Midsommar embeds the Tyk Identity Broker (TIB) to provide SSO functionality:

- **Embedded Integration:** TIB is integrated directly into the Midsommar application
- **Configuration:** Enabled via the `TIBEnabled` configuration flag
- **Security:** Secured with `TIBAPISecret` for internal communication
- **Initialization:** Initialized during API startup when enabled

The integration is configured in [api/api.go](../api/api.go):

```go
if config.TIBEnabled {
    logLevel := "info"
    if config.TestMode {
        logLevel = "debug"
    }
    ssoConfig := &services.Config{
        APISecret: config.TIBAPISecret,
        LogLevel:  logLevel,
    }
    api.ssoService = services.NewSSOService(ssoConfig, router, config.DB, service.NotificationService)
    api.ssoService.InitInternalTIB()
}
```

---

## Authentication Flow
1. **Initiation:**
   - User can initiate SSO authentication in two ways:
     - By clicking the "Log in with SSO" button on the login page (if a default profile is configured)
     - By navigating directly to the SSO login URL (`/auth/:id/:provider`)
   - System identifies the requested profile and provider

2. **Provider Authentication:**
   - User is redirected to the identity provider
   - Identity provider authenticates the user
   - Provider redirects back to the callback URL (`/auth/:id/:provider/callback`)
   - Provider redirects back to the callback URL (`/auth/:id/:provider/callback`)

3. **Callback Processing:**
   - System validates the response from the identity provider
   - User identity information is extracted from the response

4. **User Management:**
   - System checks if the user exists
   - If not, a new user is created (if allowed)
   - User information is updated with the latest data from the provider

5. **Group Assignment:**
   - System extracts group information from the provider response
   - Groups are mapped to internal Midsommar groups using the profile's mapping configuration
   - User is assigned to the appropriate groups

6. **Session Creation:**
   - System creates a new session for the authenticated user
   - User is redirected to the return URL with a valid session

---

## Supported Providers

### OpenID Connect
- Standard OAuth 2.0 and OpenID Connect flows
- Configurable endpoints for authorization, token, and userinfo
- Support for custom scopes and claims
- Skip UserInfo request option for providers that don't support it
- Customizable token validation settings

### SAML
- SP-initiated and IdP-initiated flows
- **Certificate Management:** File system-based certificate manager (currently the only supported option)
- **Important:** SAML certificates must be provided as accessible file paths in the profile configuration
- Customizable attribute mapping for user information
- Support for custom email and user ID field mapping

### LDAP
- Direct LDAP server integration
- Support for secure LDAP (LDAPS)
- Customizable search filters and attribute mapping
- Base DN and search scope configuration

### Social Providers
- Support for common social login providers (Google, GitHub, etc.)
- OAuth-based authentication
- Custom provider configuration options

---

## User Group Mapping

The SSO system maps external identity provider groups to internal Midsommar groups:

### Configuration Options
- **Custom Group Field:** Specify which field from the identity provider contains group information
- **Group Mapping:** Map external group identifiers to internal Midsommar group IDs
- **Group Separator:** Configure the separator used in multi-group values
- **Default Group:** Assign a default group for users without specific group mappings

### Implementation
The group mapping process is handled in [api/auth_handlers.go](../api/auth_handlers.go):

1. Extract group information from the identity provider response
2. Split multi-group values using the configured separator
3. Map external group IDs to internal group IDs
4. Ensure the default group is always included
5. Update the user's group memberships using a variadic parameter method

The `UpdateGroupMemberships` method in [models/user.go](../models/user.go) efficiently updates a user's group memberships using GORM's Association.Replace for optimal database operations:

```go
func (u *User) UpdateGroupMemberships(tx *gorm.DB, groupIDs ...string) error {
    return tx.Model(u).Association("Groups").Replace(groupIDs)
}
```

This implementation ensures:
- Users can be assigned to multiple groups simultaneously
- Group assignments are updated in a single database operation
- The default group is always included in the user's memberships
- Efficient removal of previous group assignments that are no longer valid

---

## API Endpoints

| Endpoint | Method | Purpose | Auth Required |
|----------|--------|---------|------------|
| /api/v1/sso-profiles | POST | Create a new SSO profile | Yes (Admin) |
| /api/v1/sso-profiles | GET | List all SSO profiles | Yes (Admin) |
| /api/v1/sso-profiles/:profile_id | GET | Get a specific SSO profile | Yes (Admin) |
| /api/v1/sso-profiles/:profile_id | PUT | Update a specific SSO profile | Yes (Admin) |
| /api/v1/sso-profiles/:profile_id | DELETE | Delete a specific SSO profile | Yes (Admin) |
| /api/v1/sso-profiles/:profile_id/use-in-login-page | POST | Set a profile as the default for the login page | Yes (Admin) |
| /login-sso-profile | GET | Get the profile configured as default for the login page | No |
| /auth/:id/:provider | GET/POST | Initiate authentication with provider | No |
| /auth/:id/:provider/callback | GET/POST | Handle provider callback | No |
| /auth/:id/saml/metadata | GET/POST | Serve SAML metadata | No |
| /sso | GET | Handle SSO authentication flow | No |
| /api/sso | POST | Handle nonce token requests | Yes (SSO Auth) |

---

## UI Components

### SSO Profiles Page
- List view of all configured SSO profiles
- Actions for creating, editing, and deleting profiles
- Option to set a profile as the default for the login page via context menu
- Confirmation dialog when setting a profile as default, explaining that only one profile can be the default
- Empty state with guidance for when no profiles exist

### SSO Profile Editor
- Monaco-based JSON editor for profile configuration with syntax highlighting and validation
- Simple navigation with back button to return to profiles list
- Supports both creation and editing of profiles in a single interface
- Automatic conversion between snake_case (API) and CamelCase (UI) formats

### Login Page Integration
- Displays a "Log in with SSO" button when a default SSO profile is configured
- Visual separator between traditional login form and SSO option
- Direct access to the configured SSO provider without requiring navigation to a separate page

### Profile Details View
- Organized sections for different aspects of the profile:
  - Basic profile details
  - Provider configuration
  - Advanced settings
  - User group mapping
- Copy-to-clipboard functionality for URLs and configuration values

---
## Configuration Requirements

### Default Login Profile Configuration
- Only one profile can be set as the default for the login page at a time
- Setting a new default profile automatically removes the default status from any previously configured profile
- The default profile is displayed on the login page with a "Log in with SSO" button
- Administrators can set a profile as default through the SSO Profiles page in the admin interface
- The default profile should be configured to provide a seamless authentication experience for users

## Future Enhancements
## Future Enhancements

1. **Wizard Interface for Profile Management**
   - Form-based wizard as an alternative to the JSON editor for creating/editing profiles

2. **Enhanced Certificate Management**
   - Custom certificate manager to upload certificates from the UI for SAML authentication
   - Store certificates in the system database instead of requiring file system paths

---
