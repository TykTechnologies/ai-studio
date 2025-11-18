# Testing Strategy for CE/ENT Split

## Overview

This document describes the testing strategy for handling Community Edition (CE) and Enterprise Edition (ENT) test suites.

## Problem

The codebase uses a factory pattern with `init()` functions to register enterprise implementations. This creates import cycle issues when tests try to import enterprise packages to trigger factory registration.

## Solution 1: TestMain with Enterprise Feature Imports (Recommended)

The preferred approach is to use a `TestMain` function in an enterprise-tagged file to import all enterprise features before tests run. This registers the factories via their `init()` functions.

### Pattern

**Enterprise TestMain File** (`testmain_enterprise_test.go`):
```go
//go:build enterprise
// +build enterprise

package mypackage

import (
    "os"
    "testing"

    // Import enterprise features to register factories before tests run
    _ "github.com/TykTechnologies/midsommar/v2/enterprise/features/budget"
    _ "github.com/TykTechnologies/midsommar/v2/enterprise/features/edge_management"
    _ "github.com/TykTechnologies/midsommar/v2/enterprise/features/group_access"
    _ "github.com/TykTechnologies/midsommar/v2/enterprise/features/licensing"
    _ "github.com/TykTechnologies/midsommar/v2/enterprise/features/marketplace_management"
    _ "github.com/TykTechnologies/midsommar/v2/enterprise/features/plugin_security"
    _ "github.com/TykTechnologies/midsommar/v2/enterprise/features/sso"
)

func TestMain(m *testing.M) {
    // Enterprise factories are now registered via init()
    os.Exit(m.Run())
}
```

**Key Points**:
- The file MUST have `//go:build enterprise` tag so it only compiles in enterprise builds
- Use blank imports (`_`) to trigger init() functions without using the packages
- Import ALL enterprise features that the package's tests might need
- The TestMain function runs BEFORE any tests, ensuring factories are registered
- In CE builds, this file doesn't compile, and tests use CE factory stubs

**Note on Import Cycles**: Some packages (like `services/`) cannot import `budget` due to circular dependencies. Exclude those imports and add a comment explaining why.

### When to Use TestMain Pattern

Use this approach when:
- ✅ Multiple tests in the package create services
- ✅ You want to avoid duplicating test code
- ✅ Tests work in CE with limited functionality (using stub implementations)
- ✅ No import cycles exist with enterprise features

### Examples of TestMain Pattern

See these files for working examples:
- [`auth/testmain_enterprise_test.go`](auth/testmain_enterprise_test.go) - Auth package
- [`services/testmain_enterprise_test.go`](services/testmain_enterprise_test.go) - Services package (excludes budget)
- [`api/testmain_enterprise_test.go`](api/testmain_enterprise_test.go) - API package
- [`proxy/testmain_enterprise_test.go`](proxy/testmain_enterprise_test.go) - Proxy package

## Solution 2: Build-Tagged Test Files (Alternative)

Use separate test files with build tags for CE and ENT versions of tests that depend on services. This approach is useful when TestMain isn't sufficient or when you want completely different test implementations.

### Pattern

For any test that creates services (which need factory registration):

**1. Enterprise Test File** (`*_enterprise_test.go`):
```go
//go:build enterprise
// +build enterprise

package mypackage

// Full test implementation with real service creation
func TestFeatureEnterprise(t *testing.T) {
    db := setupTestDB(t)
    service := services.NewService(db)  // Works because ENT factories are registered

    // ... full test logic ...
}
```

**2. Community Test File** (`*_community_test.go`):
```go
//go:build !enterprise
// +build !enterprise

package mypackage

// Placeholder test for CE build
func TestFeatureCommunity(t *testing.T) {
    assert.True(t, true, "CE test framework operational")
    t.Log("Full tests run in Enterprise Edition")
}
```

## Examples

### API Tests

- [`api/analytics_handlers_enterprise_test.go`](api/analytics_handlers_enterprise_test.go) - Full analytics tests
- [`api/analytics_handlers_community_test.go`](api/analytics_handlers_community_test.go) - CE placeholder

### Service Tests

- [`services/chat_service_enterprise_test.go`](services/chat_service_enterprise_test.go) - Full chat tests
- [`services/chat_service_community_test.go`](services/chat_service_community_test.go) - CE placeholder

## When to Use Each Pattern

### Use TestMain Pattern (Solution 1) When:
1. ✅ Multiple tests in package create `services.NewService(db)`
2. ✅ Tests should work in both CE and EE (with different behavior)
3. ✅ No enterprise-specific test logic needed
4. ✅ Package has no import cycles with enterprise features

### Use Build-Tagged Test Files (Solution 2) When:
1. ✅ Tests need completely different implementations for CE vs EE
2. ✅ Enterprise tests require features not available in CE
3. ✅ TestMain approach creates import cycles
4. ✅ Only a few tests need service creation

### Use Neither Pattern When:
1. ❌ Test only uses models directly (no service layer)
2. ❌ Test uses HTTP handlers without service creation
3. ❌ Test is pure unit test with no database
4. ❌ Test doesn't call `services.NewService()` or related factory methods

## Running Tests

### Community Edition
```bash
go test ./...                    # Runs all CE tests
go test ./api/...                # Runs CE API tests
```

### Enterprise Edition
```bash
go test -tags enterprise ./...   # Runs all ENT tests
go test -tags enterprise ./api/... # Runs ENT API tests
```

## Benefits

1. **No Import Cycles**: Enterprise packages don't need to be imported in CE builds
2. **Clean Separation**: CE and ENT test logic is clearly separated
3. **Factory Registration**: ENT tests automatically get factory registration through build system
4. **Compilation Success**: Both CE and ENT builds compile and run successfully

## Gotchas

1. **Backup Old Tests**: Move original test files to `.bak` before creating split versions
2. **Consistent Naming**: Use `*_enterprise_test.go` and `*_community_test.go` suffixes
3. **Test Coverage**: CE tests should have minimal placeholders, full tests in ENT
4. **Model Fields**: Check model structures when creating test data (field names change)

## Migration Checklist

When splitting an existing test file:

- [ ] Backup original: `mv test.go test.go.bak`
- [ ] Create `*_enterprise_test.go` with `//go:build enterprise` tag
- [ ] Create `*_community_test.go` with `//go:build !enterprise` tag
- [ ] Copy full test logic to enterprise file
- [ ] Add placeholder to community file
- [ ] Test CE build: `go test ./package/...`
- [ ] Test ENT build: `go test -tags enterprise ./package/...`
- [ ] Remove backup if successful

## Future Improvements

Consider these alternatives if build-tagged files become unwieldy:

1. **Mock Factories**: Create test-only factories that don't require imports
2. **Test Helpers**: Centralize service creation in test utilities
3. **Refactor Factories**: Move factory registration to explicit calls instead of `init()`
