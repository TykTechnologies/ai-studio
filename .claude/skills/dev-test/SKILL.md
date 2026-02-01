---
name: dev-test
description: Run unit tests, integration tests, or the full test suite for Tyk AI Studio. Use when the user wants to run tests, check test coverage, or verify code changes.
argument-hint: [all|quick|studio|gateway|frontend|plugins] [--verbose] [--coverage]
allowed-tools: Bash
---

# Run Tests

Run the test suite for Tyk AI Studio components.

## Arguments

**Test scope** (first argument):
- `all` - Run all tests (unit + integration)
- `quick` - Run only unit tests (fast feedback)
- `ci` - Run CI tests with coverage
- `studio` - AI Studio Go unit tests only
- `gateway` - Microgateway unit tests only
- `frontend` - Frontend Jest tests only
- `plugins` - Plugin unit tests only

**Options:**
- `--verbose` or `-v` - Enable verbose output
- `--coverage` or `-c` - Generate coverage report
- `--integration` or `-i` - Include integration tests (for component targets)

## Usage

| Command | Make Target |
|---------|-------------|
| `/dev-test all` | `make test-all` |
| `/dev-test quick` | `make test-quick` |
| `/dev-test ci` | `make test-ci` |
| `/dev-test studio` | `make test-studio-unit` |
| `/dev-test gateway` | `make test-microgateway-unit` |
| `/dev-test frontend` | `make test-frontend-unit` |
| `/dev-test plugins` | `make test-plugins-unit` |

**With options:**
| Command | Make Target |
|---------|-------------|
| `/dev-test all --verbose` | `make test-all TEST_VERBOSE=true` |
| `/dev-test all --coverage` | `make test-all TEST_COVERAGE=true` |
| `/dev-test studio --integration` | `make test-studio-integration` |
| `/dev-test gateway --integration` | `make test-microgateway-integration` |

## Configuration Variables

These can be passed to any test target:

| Variable | Values | Description |
|----------|--------|-------------|
| `TEST_EDITION` | `ce`, `ent` | Edition to test (default: auto-detect) |
| `TEST_VERBOSE` | `true`, `false` | Enable verbose output |
| `TEST_COVERAGE` | `true`, `false` | Generate coverage report |
| `TEST_TIMEOUT` | e.g., `30m` | Test timeout duration |

## Examples

```bash
# Run all tests
make test-all

# Quick unit tests for fast feedback
make test-quick

# Run with verbose output and coverage
make test-all TEST_VERBOSE=true TEST_COVERAGE=true

# Test specific components
make test-studio-unit
make test-microgateway-unit

# Run integration tests
make test-studio-integration
make test-microgateway-integration

# Enterprise edition tests
make test-all TEST_EDITION=ent
```

## Notes

- `test-quick` is the fastest option for verifying changes
- Integration tests may require Docker to be running
- Plugin integration and E2E tests require enterprise edition (`TEST_EDITION=ent`)
- Use `make test-help` for complete documentation
