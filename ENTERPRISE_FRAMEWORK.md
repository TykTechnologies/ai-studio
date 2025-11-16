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
- ✅ Hub-and-spoke deployment
- ✅ Cost tracking and analytics
- ✅ Plugin system

**Not Included:**
- ❌ Budget enforcement
- ❌ Budget alerts and notifications
- ❌ Advanced SSO (SAML, OIDC)
- ❌ Advanced RBAC
- ❌ Audit logging

### Enterprise Edition (ENT)

**Everything in CE, plus:**
- ✅ Budget management and enforcement
- ✅ Budget alerts (80%, 100% thresholds)
- ✅ Budget forecasting
- ✅ Advanced SSO integration
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
