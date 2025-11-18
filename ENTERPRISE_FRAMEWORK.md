# Enterprise/Community Edition Framework

This document describes the Enterprise/Community Edition framework for Tyk AI Studio (Midsommar).

## Overview

Tyk AI Studio is available in two editions:

- **Community Edition (CE)**: Open source, available to everyone
- **Enterprise Edition (ENT)**: Additional features for production deployments, private repository

The framework uses **Go build tags** and a **private git submodule** to separate enterprise features while maintaining a single codebase.

## Architecture

### Repository Structure

```
midsommar/                              # Public repository
├── .gitmodules                         # Submodule configuration
├── enterprise/                         # Private submodule (git ignored)
│   ├── .git → ai-studio-enterprise    # Points to private repo
│   ├── features/
│   │   └── budget/                    # Enterprise budget service
│   │       ├── service.go             # Full implementation
│   │       ├── init.go                # Factory registration
│   │       └── service_test.go        # Enterprise tests
│   └── go.mod                         # Enterprise module
├── services/
│   └── budget/
│       ├── interface.go               # Budget interface (public)
│       ├── factory.go                 # Factory pattern (public)
│       └── community.go               # CE stub (public)
├── microgateway/
│   └── internal/services/
│       ├── budget_service.go          # ENT impl with build tag
│       └── budget_community.go        # CE stub
└── main_enterprise.go                 # Enterprise imports (build tag)
```

### Module Dependencies

```
Main Module (github.com/TykTechnologies/midsommar/v2)
  ↓ (optional, via replace)
Enterprise Module (github.com/TykTechnologies/midsommar/v2/enterprise)
  ↓ (imports)
Main Module (no cycle!)

Microgateway Module (github.com/TykTechnologies/midsommar/microgateway)
  ↓ (via replace)
Main Module (one-way dependency)
```

**No import cycles** - Enterprise and Microgateway never import back to create cycles.

## Build System

### Automatic Edition Detection

The Makefile automatically detects which edition to build based on enterprise submodule presence:

```makefile
ENTERPRISE_EXISTS := $(shell test -f enterprise/.git && echo "yes" || echo "no")

ifeq ($(ENTERPRISE_EXISTS),yes)
    BUILD_TAGS := -tags enterprise
    EDITION := ent
else
    BUILD_TAGS :=
    EDITION := ce
endif
```

### Build Commands

#### Main Application (Midsommar)

```bash
# From root directory

# Auto-detect edition:
make build                  # Builds CE or ENT based on submodule
make build-local            # Local development build

# Force specific edition:
make build-enterprise       # Force ENT (requires submodule)

# Check current edition:
make show-edition

# Initialize enterprise:
make init-enterprise        # Requires private repo access
```

**Outputs**: `bin/midsommar-ce` or `bin/midsommar-ent`

#### Microgateway

```bash
# From microgateway/ directory

# Auto-detect edition:
make build                  # Builds CE or ENT based on submodule
make build-both             # Build server and CLI

# Force specific edition:
make build-community        # Force CE build
make build-enterprise       # Force ENT build (requires submodule)

# Check current edition:
make show-edition
```

**Outputs**: `dist/microgateway-ce` or `dist/microgateway-ent`

### Manual Build Commands

```bash
# Community Edition (no build tag)
go build -o bin/midsommar-ce
cd microgateway && go build -o dist/microgateway-ce ./cmd/microgateway

# Enterprise Edition (requires -tags enterprise)
go build -tags enterprise -o bin/midsommar-ent
cd microgateway && go build -tags enterprise -o dist/microgateway-ent ./cmd/microgateway
```

## Feature Implementation Pattern

### 1. Define Interface (Public)

```go
// services/myfeature/interface.go
package myfeature

type Service interface {
    DoSomething() error
    GetData() ([]Data, error)
}
```

### 2. Create Factory (Public)

```go
// services/myfeature/factory.go
package myfeature

type FactoryFunc func(db *gorm.DB) Service

var enterpriseFactory FactoryFunc

func RegisterEnterpriseFactory(f FactoryFunc) {
    enterpriseFactory = f
}

func NewService(db *gorm.DB) Service {
    if enterpriseFactory != nil {
        return enterpriseFactory(db)
    }
    return newCommunityService()
}

func IsEnterpriseAvailable() bool {
    return enterpriseFactory != nil
}
```

### 3. Create Community Stub (Public)

```go
// services/myfeature/community.go
package myfeature

type communityService struct{}

func newCommunityService() Service {
    return &communityService{}
}

func (s *communityService) DoSomething() error {
    return nil  // No-op or return error
}

func (s *communityService) GetData() ([]Data, error) {
    return nil, errors.New("Enterprise feature")
}
```

### 4. Create Enterprise Implementation (Private Submodule)

```go
// enterprise/features/myfeature/service.go
//go:build enterprise
// +build enterprise

package myfeature

import "github.com/TykTechnologies/midsommar/v2/services/myfeature"

type enterpriseService struct {
    db *gorm.DB
}

func NewEnterpriseService(db *gorm.DB) myfeature.Service {
    return &enterpriseService{db: db}
}

func (s *enterpriseService) DoSomething() error {
    // Full implementation
}

func (s *enterpriseService) GetData() ([]Data, error) {
    // Full implementation
}
```

### 5. Register Factory (Private Submodule)

```go
// enterprise/features/myfeature/init.go
//go:build enterprise
// +build enterprise

package myfeature

import "github.com/TykTechnologies/midsommar/v2/services/myfeature"

func init() {
    myfeature.RegisterEnterpriseFactory(NewEnterpriseService)
}
```

### 6. Import in Main (Public with Build Tag)

```go
// main_enterprise.go
//go:build enterprise
// +build enterprise

package main

import (
    _ "github.com/TykTechnologies/midsommar/v2/enterprise/features/myfeature"
)
```

## Testing

### Running Tests

```bash
# Community Edition tests (without enterprise)
go test ./...
cd microgateway && go test ./...

# Enterprise Edition tests (with enterprise)
go test -tags enterprise ./...
cd enterprise && go test -tags enterprise ./...
cd microgateway && go test -tags enterprise ./...
```

### Test Organization

- **Public repo tests**: Test CE functionality and interface compliance
- **Enterprise repo tests**: Test ENT-specific logic and features
- **Integration tests**: Use build tags for ENT-only scenarios

Example:
```go
//go:build enterprise
// +build enterprise

package proxy

func TestBudgetEnforcement(t *testing.T) {
    // Test budget enforcement
}
```

## Feature Comparison

### Community Edition (CE)

**Included:**
- ✅ LLM proxy gateway
- ✅ Chat interface
- ✅ Tool integration
- ✅ User management
- ✅ Basic RBAC
- ✅ Hub-and-spoke deployment (single "default" namespace)
- ✅ Edge gateway management
- ✅ Configuration synchronization
- ✅ Cost tracking and analytics
- ✅ Plugin system (basic security)
- ✅ Plugin path whitelisting
- ✅ Plugin checksum validation
- ✅ Single marketplace (Tyk official)

**Not Included:**
- ❌ Budget enforcement
- ❌ Budget alerts and notifications
- ❌ SSO (SAML, OIDC, LDAP, Social)
- ❌ Multi-tenant namespaces
- ❌ Namespace-based operations
- ❌ Plugin GRPC host whitelisting
- ❌ Plugin OCI signature verification
- ❌ Multiple marketplace sources
- ❌ Custom marketplace management
- ❌ Advanced RBAC
- ❌ Audit logging

### Enterprise Edition (ENT)

**Everything in CE, plus:**
- ✅ Budget management and enforcement
- ✅ Budget alerts (80%, 100% thresholds)
- ✅ Budget forecasting
- ✅ SSO integration (SAML, OIDC, LDAP, Social)
- ✅ Multi-provider SSO support
- ✅ User provisioning via SSO
- ✅ Group mapping from IdP
- ✅ Multi-tenant namespaces (unlimited)
- ✅ Namespace-based filtering and operations
- ✅ Per-namespace configuration isolation
- ✅ Namespace management UI
- ✅ Plugin GRPC host whitelisting (network security)
- ✅ Plugin OCI signature verification (supply chain security)
- ✅ Multiple marketplace sources
- ✅ Custom marketplace management UI
- ✅ Marketplace URL validation
- ✅ Per-marketplace sync control
- ✅ Advanced RBAC
- ✅ Audit logging
- ✅ Priority support

## Budget Feature Specifics

### How Budget Works

**Community Edition:**
- ✅ **Cost tracking**: Records LLM usage costs
- ✅ **Analytics**: View spending in dashboard
- ❌ **Budgets**: Cannot set monthly limits
- ❌ **Enforcement**: No request blocking
- ❌ **Alerts**: No notifications

**Enterprise Edition:**
- ✅ **Everything in CE**, plus:
- ✅ **Budget limits**: Set monthly budgets per app/LLM
- ✅ **Real-time enforcement**: Blocks requests when over budget
- ✅ **Email alerts**: Notifications at 80% and 100%
- ✅ **Budget analysis**: Automatic threshold monitoring
- ✅ **Edge enforcement**: Microgateway enforces locally

### Implementation Details

**Main App (Midsommar):**
- Interface: `services/budget/interface.go`
- CE Stub: `services/budget/community.go` (allows all requests)
- ENT Impl: `enterprise/features/budget/service.go` (988 lines)

**Microgateway:**
- Interface: `internal/services/interfaces.go` (BudgetServiceInterface)
- CE Stub: `internal/services/budget_community.go` (no enforcement)
- ENT Impl: `internal/services/budget_service.go` (build tag: `//go:build enterprise`)

## SSO Feature Specifics

### How SSO Works

**Community Edition:**
- ❌ **SSO Authentication**: No SSO support
- ❌ **Profile Management**: Cannot configure SSO providers
- ❌ **User Provisioning**: No automatic user creation via SSO
- 🔒 **API Endpoints**: Return 402 Payment Required
- 🔒 **Admin UI**: SSO configuration pages hidden

**Enterprise Edition:**
- ✅ **Everything in CE**, plus:
- ✅ **Multiple Protocols**: OIDC, SAML 2.0, LDAP, Social (Google, GitHub, etc.)
- ✅ **Multi-Provider**: Configure multiple SSO providers simultaneously
- ✅ **User Provisioning**: Automatic user creation on first SSO login
- ✅ **Group Mapping**: Map IdP groups to Tyk AI Studio groups
- ✅ **Profile Management**: Full CRUD via Admin UI and API
- ✅ **Login Page Integration**: SSO button appears when profile configured

### Implementation Details

**Main App (Midsommar):**
- Interface: `services/sso/interface.go` - SSO service interface
- Types: `services/sso/types.go` - Shared types (Config, Nonce, etc.)
- Factory: `services/sso/factory.go` - Factory pattern
- CE Stub: `services/sso/community.go` - Returns enterprise-only errors
- ENT Impl: `enterprise/features/sso/service.go` - Full TIB integration

**API Handlers:**
- CE Handlers: `api/sso_handlers_community.go` (build tag: `!enterprise`)
- ENT Handlers: `api/sso_handlers_enterprise.go` (build tag: `enterprise`)
- Profile Handlers (CE): `api/profile_handlers_community.go` (402 responses)
- Profile Handlers (ENT): `api/profile_handlers_enterprise.go` (full CRUD)

**Database Models (Public):**
- `models/tib_profiles.go` - SSO profile configurations
- `models/tib_kv_store.go` - Nonce token storage
- `models/tib_backend_store.go` - TIB backend integration
- Models remain in public repo for upgrade path compatibility

**Frontend:**
- Login Page: `ui/admin-frontend/src/portal/pages/Login.js`
  - SSO button gated by `tibEnabled` from `/auth/config`
  - Shows when ENT and profile configured
- Admin UI: `ui/admin-frontend/src/admin/pages/SSOProfiles.js`
  - Routes conditionally rendered based on `ShowSSOConfig` permission
  - Full profile management (CRUD, set default, etc.)

**Supported Providers:**
- **OIDC**: Standard OAuth 2.0 / OpenID Connect flows
- **SAML 2.0**: SP-initiated and IdP-initiated, metadata endpoint
- **LDAP**: Direct LDAP server integration with custom filters
- **Social**: OAuth-based (Google, GitHub, custom providers)

## Plugin Security Feature Specifics

### How Plugin Security Works

**Community Edition:**
- ✅ **Path Whitelisting**: Validates plugin paths against allowed directories
- ✅ **Checksum Validation**: SHA256 hash verification of plugin files
- ❌ **GRPC Host Whitelisting**: No internal network protection (allows all hosts)
- ❌ **OCI Signature Verification**: No Cosign verification (skips signatures)
- ⚠️  **Reduced Security**: Logs warnings about missing enterprise features

**Enterprise Edition:**
- ✅ **Everything in CE**, plus:
- ✅ **GRPC Host Whitelisting**: Blocks plugins from targeting internal IPs (10.x, 192.168.x, 127.x, ::1, etc.)
- ✅ **OCI Signature Verification**: Cosign-based manifest signature checking
- ✅ **Keyless Signing**: Support for OIDC-based keyless verification
- ✅ **Policy-Based Verification**: Custom policy file support
- ✅ **Multiple Public Keys**: Support for multiple signing keys (numbered, named, file-based)
- 🔒 **Network Security**: Protection against SSRF attacks via malicious plugins
- 🔒 **Supply Chain Security**: Ensures plugins come from trusted sources

### Implementation Details

**Main App (Midsommar):**
- Interface: [services/plugin_security/interface.go](../services/plugin_security/interface.go) - Security service interface
- Types: [services/plugin_security/types.go](../services/plugin_security/types.go) - Shared types
- Errors: [services/plugin_security/errors.go](../services/plugin_security/errors.go) - Security errors
- Factory: [services/plugin_security/factory.go](../services/plugin_security/factory.go) - Factory pattern
- CE Stub: [services/plugin_security/community.go](../services/plugin_security/community.go) - No-op security (logs warnings)
- ENT Impl: `enterprise/features/plugin_security/service.go` - Full security enforcement

**Microgateway:**
- Uses same interface from `services/plugin_security/`
- Initialized in `internal/services/container.go`
- Applied via `internal/api/handlers/validation.go`

**Enterprise Components:**
- GRPC Validator: `enterprise/features/plugin_security/grpc_validator.go`
  - Internal IP detection (CIDR-based)
  - IPv4 and IPv6 support
  - Localhost pattern matching
- Signature Verifier: `enterprise/features/plugin_security/signature_verifier.go`
  - Cosign CLI integration
  - Public key resolution (numbered, named, file-based)
  - Temporary PEM file handling
  - Bundle and policy verification

**OCI Client Integration:**
- Client: [pkg/ociplugins/client.go](../pkg/ociplugins/client.go)
  - `SetSecurityService()` method for ENT integration
  - Falls back to built-in verifier in CE mode
  - Uses enterprise service when available in ENT mode

**API Integration:**
- AI Studio: [api/validation.go](../api/validation.go) - Uses security service for GRPC validation
- Microgateway: `microgateway/internal/api/handlers/validation.go` - Same pattern

**Security Checks:**

| Security Feature | CE Behavior | ENT Behavior |
|-----------------|-------------|--------------|
| Path Whitelisting | ✅ Enforced | ✅ Enforced |
| Checksum Validation | ✅ Enforced | ✅ Enforced |
| GRPC Host Whitelisting | ⚠️ Bypassed (logs warning) | 🔒 Enforced (blocks internal IPs) |
| OCI Signature Verification | ⚠️ Skipped (logs warning) | 🔒 Enforced (Cosign verification) |

**Development Bypass:**
- `ALLOW_INTERNAL_NETWORK_ACCESS=true` - Bypasses GRPC host whitelisting in ENT (development only)

## Marketplace Management Feature Specifics

### How Marketplace Management Works

**Community Edition:**
- ✅ **Single Marketplace**: Access to official Tyk AI Studio plugin marketplace
- ✅ **Automatic Sync**: Hourly synchronization of plugin catalog
- ✅ **Plugin Browse & Install**: Full access to marketplace plugins
- ✅ **Update Checking**: Automatic update notifications for installed plugins
- ❌ **Custom Marketplaces**: Cannot add additional marketplace sources
- ❌ **Marketplace Management UI**: No admin interface for marketplace configuration
- ⚠️  **Limited Flexibility**: Restricted to single official marketplace

**Enterprise Edition:**
- ✅ **Everything in CE**, plus:
- ✅ **Multiple Marketplaces**: Configure multiple custom marketplace sources
- ✅ **Marketplace Management UI**: Full admin interface for marketplace CRUD operations
- ✅ **URL Validation**: Pre-flight validation of marketplace URLs before adding
- ✅ **Per-Marketplace Control**: Individual activation/deactivation of marketplaces
- ✅ **Default Marketplace**: Set any marketplace as the default
- ✅ **Source Tracking**: Plugins show which marketplace they came from
- 🔒 **Flexibility**: Support for internal/private plugin marketplaces
- 🔒 **Governance**: Control which plugin sources are available to users

### Implementation Details

**Service Layer:**
- Interface: `services/marketplace_management/interface.go` - Management interface
- Types: `services/marketplace_management/types.go` - ValidationResult, MarketplaceUpdate
- Errors: `services/marketplace_management/errors.go` - Management errors
- Factory: `services/marketplace_management/factory.go` - Factory pattern
- CE Stub: `services/marketplace_management/community.go` - Returns enterprise-only errors
- ENT Impl: `enterprise/features/marketplace_management/service.go` - Full CRUD operations

**API Endpoints:**
- CE Handlers: `api/marketplace_admin_handlers_community.go` (build tag: `!enterprise`) - 403 responses
- ENT Handlers: `api/marketplace_admin_handlers_enterprise.go` (build tag: `enterprise`) - Full CRUD
- Routes: `/api/v1/admin/marketplaces/*` (admin-only)

**Database Models (Public):**
- `models/marketplace.go` - MarketplaceIndex, MarketplacePlugin tables
- `MarketplaceIndex.IsDefault` - Marks the default marketplace
- `MarketplaceIndex.IsActive` - Controls sync activation
- `MarketplacePlugin.SyncedFromURL` - Tracks plugin source

**Management Operations:**

| Operation | CE Behavior | ENT Behavior |
|-----------|-------------|--------------|
| Add Marketplace | ❌ 403 Forbidden | ✅ Full validation & creation |
| Remove Marketplace | ❌ 403 Forbidden | ✅ Deletion (except default) |
| Set Default | ❌ 403 Forbidden | ✅ Changes default marketplace |
| Activate/Deactivate | ❌ 403 Forbidden | ✅ Toggles sync status |
| List Marketplaces | ✅ Shows default only | ✅ Shows all marketplaces |
| Validate URL | ❌ 403 Forbidden | ✅ Pre-flight validation |

**Configuration:**
- `MARKETPLACE_INDEX_URL` - Default marketplace URL (both CE & ENT)
- `MARKETPLACE_SYNC_INTERVAL` - Sync frequency (default: 1 hour)
- `MARKETPLACE_ENABLED` - Enable/disable marketplace feature

**Admin API Endpoints (ENT Only):**
```
POST   /api/v1/admin/marketplaces          # Add new marketplace
GET    /api/v1/admin/marketplaces          # List all marketplaces
GET    /api/v1/admin/marketplaces/:id      # Get specific marketplace
PUT    /api/v1/admin/marketplaces/:id      # Update marketplace properties
DELETE /api/v1/admin/marketplaces/:id      # Remove marketplace
POST   /api/v1/admin/marketplaces/validate # Validate URL before adding
POST   /api/v1/admin/marketplaces/:id/sync # Trigger manual sync
```

**Frontend Integration (ENT Only):**
- Admin UI: `ui/admin-frontend/src/admin/pages/MarketplaceSettings.js`
  - List all configured marketplaces
  - Add/remove marketplace sources
  - Set default marketplace
  - Activate/deactivate marketplaces
  - View sync status and plugin counts
- Navigation: Conditional menu item based on enterprise availability
- Plugin Cards: Show source marketplace indicator

**Security & Validation:**
- URL validation (HTTPS required for production)
- Accessibility check before adding
- Index format validation (index.yaml structure)
- Cannot remove default Tyk marketplace
- Cannot deactivate default marketplace
- Admin-only access to management endpoints

**Upgrade Path:**
- CE → ENT: Existing marketplace continues to work, can add more
- ENT → CE: Extra marketplaces remain in database (read-only, cannot manage)

## Hub-and-Spoke Multi-Tenant Feature Specifics

### How Hub-and-Spoke Works

**Community Edition:**
- ✅ **Hub-and-Spoke Architecture**: Full support for edge gateway deployment
- ✅ **Edge Registration**: Microgateways can register with control plane
- ✅ **Configuration Synchronization**: Real-time config push to all edges
- ✅ **Global Reload**: Single button to reload all edge gateways
- ✅ **Individual Edge Reload**: Reload specific edge instances
- ✅ **Edge Management UI**: View and manage connected edge gateways
- ✅ **Single Namespace**: All edges in "default" namespace (silent enforcement)
- ❌ **Multi-Tenant Namespaces**: Cannot create additional namespaces
- ❌ **Namespace Selector**: Hidden in UI
- ❌ **Namespace Management APIs**: Return 402 Payment Required
- ❌ **Per-Namespace Operations**: No namespace-based filtering or reloading

**Enterprise Edition:**
- ✅ **Everything in CE**, plus:
- ✅ **Multi-Tenant Namespaces**: Unlimited custom namespaces
- ✅ **Namespace Selector**: Visible in edge gateway UI
- ✅ **Namespace-Based Filtering**: View edges by namespace
- ✅ **Per-Namespace Reload**: Reload all edges in specific namespace
- ✅ **Namespace Statistics API**: Edge count and health per namespace
- ✅ **Complete Isolation**: Full namespace-based configuration isolation

### Implementation Details

**Control Plane (AI Studio):**
- Service: `services/edge_management/community.go` (CE: forces "default")
- Service: `enterprise/features/edge_management/service.go` (ENT: accepts all)
- gRPC: `grpc/control_server.go` uses EdgeManagementService
- API CE: `api/edge_handlers_community.go` (no namespace field, 402 for namespace APIs)
- API ENT: `api/edge_handlers_enterprise.go` (full namespace support)

**Edge Gateway (Microgateway):**
- Config: `EDGE_NAMESPACE` environment variable
- CE control plane: Silently overrides to "default" (no warning logs)
- ENT control plane: Accepts namespace as-is

**Frontend:**
- Feature flag: `hub_spoke_multi_tenant` from `/common/system` endpoint
- CE: Namespace selector hidden, namespace column removed from table
- ENT: Full namespace selector and per-namespace operations visible

**API Endpoints:**

| Endpoint | CE Behavior | ENT Behavior |
|----------|-------------|--------------|
| `GET /api/v1/edges` | ✅ Works (no namespace field) | ✅ Works (includes namespace field, accepts ?namespace= filter) |
| `GET /api/v1/edges/:id` | ✅ Works (no namespace field) | ✅ Works (includes namespace field) |
| `POST /api/v1/edges/:id/reload` | ✅ Works | ✅ Works |
| `POST /api/v1/edges/reload-all` | ✅ Works (reloads all edges) | ✅ Works (reloads all namespaces) |
| `DELETE /api/v1/edges/:id` | ✅ Works | ✅ Works |
| `GET /api/v1/namespaces` | ❌ 402 Payment Required | ✅ List all namespaces |
| `GET /api/v1/namespaces/:ns/stats` | ❌ 402 Payment Required | ✅ Get namespace stats |
| `POST /api/v1/namespaces/:ns/reload` | ❌ 402 Payment Required | ✅ Reload specific namespace |
| `GET /api/v1/namespaces/:ns/edges` | ❌ 402 Payment Required | ✅ Get edges in namespace |

**Architecture Pattern:**
```
CE:  Edge (namespace: "custom") → Control Plane → Silently forced to "default"
ENT: Edge (namespace: "custom") → Control Plane → Accepted as "custom"
```

**Database Schema:**
- `edge_instances.namespace` - Namespace field (indexed)
- `apps.namespace` - App namespace for routing
- `llms.namespace` - LLM namespace for routing
- Query pattern: `WHERE (namespace = '' OR namespace = ?)` - global + tenant-specific

**Key Design Decision:**
- No warning logs in CE when forcing namespace to "default"
- Silent enforcement provides clean user experience
- Backend handles normalization transparently

**Upgrade Path:**
- CE → ENT: All edges remain in "default", can now create additional namespaces
- ENT → CE: Edges keep their namespaces in DB but all forced to "default" at runtime

## Enterprise Submodule Workflow

### For Developers WITH Enterprise Access

```bash
# Initial setup
git clone git@github.com:TykTechnologies/midsommar.git
cd midsommar
make init-enterprise        # Initialize private submodule

# Build enterprise edition
make build                  # Auto-detects ENT

# Work on enterprise features
cd enterprise
git checkout -b feature/my-enterprise-feature
# Make changes to enterprise code
git add .
git commit -m "Add feature"
git push origin feature/my-enterprise-feature

# Update parent repo submodule reference
cd ..
git add enterprise
git commit -m "Update enterprise submodule"
git push
```

### For Developers WITHOUT Enterprise Access (CE Only)

```bash
# Clone public repo
git clone git@github.com:TykTechnologies/midsommar.git
cd midsommar

# Build community edition (no submodule needed)
make build                  # Auto-builds CE

# Work on public features
git checkout -b feature/my-ce-feature
# Make changes to public code
git add .
git commit -m "Add feature"
git push origin feature/my-ce-feature
```

### Updating Enterprise Submodule

```bash
# Pull latest enterprise changes
make update-enterprise

# This updates the submodule to latest commit
# Commit the reference update:
git commit -m "Update enterprise submodule to latest"
```

## Security

### Pre-commit Hook

A pre-commit hook prevents accidental commits of enterprise code to the public repository:

**Location**: `.git/hooks/pre-commit`

```bash
# Checks for enterprise code patterns
# Blocks commits containing:
# - Files in enterprise/ directory
# - Files with .enterprise extension
# - Files with -ent- or -ENT- in name
```

### .gitignore Configuration

```gitignore
# Enterprise submodule - ignore contents but git tracks the reference
/enterprise/*

# Keep submodule config and git reference visible
!/.gitmodules
!/enterprise

# Prevent accidental enterprise commits
*-ent-*
*-ENT-*
*.enterprise
/bin/*-ent
/bin/midsommar-ent*
/bin/mgw-ent*
```

## CI/CD

### GitHub Actions Example

```yaml
# Community Edition CI (runs on all PRs)
name: Build CE
on: [push, pull_request]
jobs:
  build-ce:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
        # Do NOT init submodules
      - name: Build CE
        run: make build
      - name: Test CE
        run: go test ./...

# Enterprise Edition CI (private or with secrets)
name: Build ENT
on: [push]
jobs:
  build-ent:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
        with:
          submodules: 'recursive'
          token: ${{ secrets.ENTERPRISE_PAT }}
      - name: Build ENT
        run: make build
      - name: Test ENT
        run: go test -tags enterprise ./...
```

## Troubleshooting

### "Enterprise submodule not initialized"

```bash
# You don't have access to the private repository
# Contact: enterprise@tyk.io
```

### "Import cycle not allowed"

- Check that enterprise code doesn't import packages that import it back
- Use interfaces and factory patterns
- Keep dependencies one-way: Enterprise → Main, never Main → Enterprise

### CE Build Fails

```bash
# Remove enterprise directory and try again
mv enterprise enterprise.backup
make build
# Should build CE successfully
```

### ENT Build Fails

```bash
# Ensure submodule is initialized
make init-enterprise
make build
```

### Tests Fail in CE

- Ensure enterprise-only tests have `//go:build enterprise` tag
- CE tests should not depend on enterprise features
- Use interface mocks in CE tests

## Adding New Enterprise Features

Follow this checklist when adding a new enterprise feature:

### Planning
- [ ] Define clear interface for the feature
- [ ] Decide what CE gets (if anything)
- [ ] Plan database schema (nullable fields for CE compatibility)

### Implementation
- [ ] Create interface in public repo: `services/myfeature/interface.go`
- [ ] Create factory pattern: `services/myfeature/factory.go`
- [ ] Create CE stub: `services/myfeature/community.go`
- [ ] Implement in enterprise: `enterprise/features/myfeature/service.go`
- [ ] Add factory registration: `enterprise/features/myfeature/init.go`
- [ ] Import in main: Update `main_enterprise.go`

### API Layer (if applicable)
- [ ] Create ENT handlers with `//go:build enterprise` tag
- [ ] Create CE handlers that return 402 Payment Required
- [ ] Update router to use conditional registration

### Frontend (if applicable)
- [ ] Add feature detection API endpoint
- [ ] Update FeatureContext with new feature flag
- [ ] Conditionally render UI components
- [ ] Show "Upgrade to Enterprise" banner in CE

### Testing
- [ ] Write ENT tests with `//go:build enterprise` tag
- [ ] Write CE tests for stub functionality
- [ ] Test CE build without submodule
- [ ] Test ENT build with submodule

### Documentation
- [ ] Update this document
- [ ] Update feature specs in `features/` directory
- [ ] Add to README feature comparison table

## Budget Feature Migration (Reference Implementation)

The budget feature was the first feature migrated to this framework. It serves as a reference implementation.

### What Was Migrated

**Main App:**
- `services/budget_service.go` (987 lines) → `enterprise/features/budget/service.go`
- Interface: `services/budget/interface.go`
- Factory: `services/budget/factory.go`
- CE Stub: `services/budget/community.go`

**Microgateway:**
- `internal/services/budget_service.go` tagged with `//go:build enterprise`
- CE Stub: `internal/services/budget_community.go`
- API handlers: Tagged with build tags
- Router: Conditional route registration

### Code Metrics

- **Public repo**: -1,400 lines (enterprise code removed)
- **Public repo**: +150 lines (interfaces/stubs added)
- **Private repo**: +1,400 lines (enterprise implementations)
- **Net result**: CE binary is cleaner, ENT has full features

## Edition Detection API

Applications can detect which edition they're running:

```bash
# Endpoint
GET /api/v1/system/edition

# Response (CE)
{
  "edition": "community",
  "features": {
    "budget": false,
    "sso": false,
    "audit": false
  }
}

# Response (ENT)
{
  "edition": "enterprise",
  "features": {
    "budget": true,
    "sso": true,
    "audit": true
  }
}
```

## Frontend Integration

```javascript
// src/contexts/FeatureContext.js
const { features, edition } = useFeatures();

// Conditional rendering
{features.budget ? (
  <BudgetWidget />
) : (
  <UpgradeBanner
    feature="Budget Management"
    edition={edition}
  />
)}
```

## Deployment

### Docker Images

```dockerfile
# Community Edition
FROM golang:1.24 AS builder
COPY . .
RUN make build

FROM alpine:latest
COPY --from=builder /app/bin/midsommar-ce /usr/local/bin/
ENTRYPOINT ["midsommar-ce"]

# Enterprise Edition (requires submodule access)
FROM golang:1.24 AS builder
COPY . .
RUN git submodule update --init --recursive
RUN make build

FROM alpine:latest
COPY --from=builder /app/bin/midsommar-ent /usr/local/bin/
ENTRYPOINT ["midsommar-ent"]
```

### Binary Distribution

```bash
# Release process
make build-all              # Build all architectures

# Create tarballs
tar -czf midsommar-ce-v2.0.0-linux-amd64.tar.gz bin/midsommar-ce-amd64
tar -czf midsommar-ent-v2.0.0-linux-amd64.tar.gz bin/midsommar-ent-amd64
```

## Upgrade Path

### From CE to ENT

1. Stop CE services
2. Backup database
3. Deploy ENT binaries (same database schema)
4. Start ENT services
5. Configure enterprise features (budgets, SSO, etc.)

**No data migration needed** - Database schema is compatible.

### From ENT to CE (Downgrade)

1. Stop ENT services
2. Deploy CE binaries
3. Start CE services

**Note**: Enterprise features will be disabled but data is preserved.

## Frequently Asked Questions

### Q: Can I mix CE hub with ENT gateway?
**A**: Yes, but ENT features won't work. The gateway respects the hub's edition.

**Recommended combinations:**
- ✅ CE hub + CE gateway
- ✅ ENT hub + ENT gateway
- ⚠️ ENT hub + CE gateway (edge can't enforce budgets)
- ⚠️ CE hub + ENT gateway (no enterprise features to use)

### Q: How do I know which edition I'm running?
```bash
# Check binary name
ls bin/
# midsommar-ce or midsommar-ent

# Or check at runtime
curl http://localhost:3000/api/v1/system/edition
```

### Q: Can I use enterprise features without a license key?
**A**: Yes! Edition is determined at **build time**, not runtime. If you have access to the private repository and build with `-tags enterprise`, you get all features. This is honor-system based for self-hosted deployments.

### Q: How do I get enterprise access?
**A**: Contact enterprise@tyk.io for access to the private repository.

### Q: What happens to my data if I downgrade from ENT to CE?
**A**: Data is preserved but enterprise features are disabled. Budget configurations remain in the database but aren't enforced.

### Q: Can I contribute to enterprise features?
**A**: Enterprise features are in a private repository. Public contributions go to CE features. Enterprise team members can contribute to both.

## Support

- **Community Edition**: GitHub issues, community forum
- **Enterprise Edition**: Priority support via enterprise support channel
- **Enterprise Sales**: enterprise@tyk.io

## License

- **Community Edition**: Apache 2.0 (open source)
- **Enterprise Edition**: Proprietary license (commercial)

---

**Last Updated**: November 2025
**Framework Version**: 1.0
