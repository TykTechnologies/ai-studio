# Object Hooks Test Plugin

A comprehensive test plugin that implements all object hook types (24 combinations) with **automated UI testing** and configurable behavior for testing the AI Studio object hooks system.

## Features

### 🧪 Automated Test Runner
- **One-Click Testing**: Click "Run All Tests" button to automatically test all 24 hook combinations
- **Visual Dashboard**: Real-time test results with pass/fail indicators, progress bar, and duration tracking
- **Coverage Matrix**: Interactive table showing test status for each object type × hook type combination
- **Filter Results**: View all tests, only passed, or only failed tests
- **No Manual Work**: Tests run automatically through the UI - no need to manually create/update/delete objects

### Hook Testing Features
- **All Object Types**: Tests hooks for LLM, Datasource, Tool, and User objects
- **All Hook Types**: Tests before_create, after_create, before_update, after_update, before_delete, after_delete
- **Configurable Behavior**: Four modes per hook type:
  - `allow`: Pass through (default)
  - `reject`: Block the operation with custom message
  - `modify`: Modify object fields
  - `metadata`: Add metadata to objects
- **Detailed Logging**: Logs all hook invocations to stderr for verification

## Quick Start

### 1. Build

```bash
cd examples/plugins/studio/hook-test-plugin
go build -o hook-test-plugin
```

## Installation

### Via CLI (mgw command)

```bash
# Create plugin
./mgw plugin create \
  --name "Object Hooks Test Plugin" \
  --command "/path/to/hook-test-plugin" \
  --config '{"enable_logging": true}'

# Enable plugin
./mgw plugin enable --id <plugin-id>
```

### Via API

```bash
# Install plugin
curl -X POST http://localhost:3000/api/v1/plugins \
  -H "Authorization: Bearer <token>" \
  -F "name=Object Hooks Test Plugin" \
  -F "command=/path/to/hook-test-plugin" \
  -F "config={\"enable_logging\": true}"
```

### 3. Access the Test Runner UI

1. Navigate to AI Studio in your browser
2. Go to the Plugin Test Dashboard section
3. Find "Object Hooks Test Runner" in the list
4. Click "Run All Tests" to automatically test all 24 hook combinations

The UI will show:
- Real-time progress bar
- Pass/fail counts
- Test duration
- Coverage matrix showing status of each hook
- Detailed results for each test

## Using the Automated Test Runner

The test runner provides a visual interface for comprehensive testing:

1. **Run All Tests**: Single button to test all 24 combinations automatically
2. **View Results**:
   - Summary cards show total tests, passed, failed, and duration
   - Coverage matrix shows which hooks passed (✓) or failed (✗)
   - Detailed results list shows messages and errors for each test
3. **Filter Results**: Click "All", "Passed", or "Failed" to filter the results view
4. **Clear Results**: Reset the dashboard to run tests again

The test runner creates synthetic test objects for each type and verifies that:
- The hook is called correctly
- The hook can allow or reject operations
- The hook can modify objects
- The hook can add metadata
- All object types (llm, datasource, tool, user) work correctly
- All hook types (before/after create/update/delete) work correctly

## Manual Configuration Examples

### Example 1: Test Rejection

Configure to reject all create operations:

```json
{
  "enable_logging": true,
  "before_create": {
    "mode": "reject",
    "rejection_reason": "Testing rejection behavior"
  }
}
```

Test:
```bash
# Try to create an LLM - should be rejected
curl -X POST http://localhost:3000/api/v1/llms \
  -H "Authorization: Bearer <token>" \
  -d '{"name": "test-llm", "api_endpoint": "https://api.example.com"}'
```

Expected: Operation rejected with message "Testing rejection behavior"

### Example 2: Test Modification

Configure to modify names on create:

```json
{
  "enable_logging": true,
  "before_create": {
    "mode": "modify",
    "modify_field": "name",
    "modify_value": "hook-modified-name"
  }
}
```

Test:
```bash
# Create an LLM with name "original-name"
curl -X POST http://localhost:3000/api/v1/llms \
  -H "Authorization: Bearer <token>" \
  -d '{"name": "original-name", "api_endpoint": "https://api.example.com"}'
```

Expected: LLM created with name "hook-modified-name"

### Example 3: Test Metadata

Configure to add metadata on all operations:

```json
{
  "enable_logging": true,
  "before_create": {
    "mode": "metadata",
    "metadata_key": "validated_by_test",
    "metadata_value": "true"
  },
  "after_create": {
    "mode": "metadata",
    "metadata_key": "created_timestamp",
    "metadata_value": "2025-01-07T12:00:00Z"
  }
}
```

Test:
```bash
# Create an LLM and check metadata
curl -X POST http://localhost:3000/api/v1/llms \
  -H "Authorization: Bearer <token>" \
  -d '{"name": "test-llm", "api_endpoint": "https://api.example.com"}'
```

Expected: LLM created with metadata containing both keys from before and after hooks

### Example 4: Comprehensive Test

Configure different behavior for each hook type:

```json
{
  "enable_logging": true,
  "before_create": {
    "mode": "metadata",
    "metadata_key": "before_create",
    "metadata_value": "executed"
  },
  "after_create": {
    "mode": "metadata",
    "metadata_key": "after_create",
    "metadata_value": "executed"
  },
  "before_update": {
    "mode": "modify",
    "modify_field": "short_description",
    "modify_value": "Modified by update hook"
  },
  "after_update": {
    "mode": "metadata",
    "metadata_key": "last_updated_by_hook",
    "metadata_value": "true"
  },
  "before_delete": {
    "mode": "allow"
  },
  "after_delete": {
    "mode": "allow"
  }
}
```

## Testing Workflow

### 1. Test All Create Hooks

```bash
# Configure for create testing
# Set before_create and after_create with different modes

# Test LLM creation
curl -X POST http://localhost:3000/api/v1/llms ...

# Test Datasource creation
curl -X POST http://localhost:3000/api/v1/datasources ...

# Test Tool creation
curl -X POST http://localhost:3000/api/v1/tools ...

# Test User creation
curl -X POST http://localhost:3000/api/v1/users ...
```

### 2. Test All Update Hooks

```bash
# Configure for update testing
# Set before_update and after_update with different modes

# Test LLM update
curl -X PUT http://localhost:3000/api/v1/llms/{id} ...

# Test Datasource update
curl -X PUT http://localhost:3000/api/v1/datasources/{id} ...

# And so on...
```

### 3. Test All Delete Hooks

```bash
# Configure for delete testing
# Set before_delete and after_delete with different modes

# Test LLM deletion
curl -X DELETE http://localhost:3000/api/v1/llms/{id} ...

# Test Datasource deletion
curl -X DELETE http://localhost:3000/api/v1/datasources/{id} ...

# And so on...
```

### 4. Monitor Logs

Watch stderr output to see hook invocations:

```bash
tail -f /path/to/logs | grep hook-test-plugin
```

Expected output:
```
[hook-test-plugin] Hook invoked: object_type=llm, hook_type=before_create, object_id=0
[hook-test-plugin] Adding metadata: before_create=executed
[hook-test-plugin] Hook invoked: object_type=llm, hook_type=after_create, object_id=123
[hook-test-plugin] Adding metadata: after_create=executed
```

## Coverage Matrix

This plugin tests all 24 combinations:

| Object Type | Hook Type      | Tested |
|-------------|----------------|--------|
| llm         | before_create  | ✓      |
| llm         | after_create   | ✓      |
| llm         | before_update  | ✓      |
| llm         | after_update   | ✓      |
| llm         | before_delete  | ✓      |
| llm         | after_delete   | ✓      |
| datasource  | before_create  | ✓      |
| datasource  | after_create   | ✓      |
| datasource  | before_update  | ✓      |
| datasource  | after_update   | ✓      |
| datasource  | before_delete  | ✓      |
| datasource  | after_delete   | ✓      |
| tool        | before_create  | ✓      |
| tool        | after_create   | ✓      |
| tool        | before_update  | ✓      |
| tool        | after_update   | ✓      |
| tool        | before_delete  | ✓      |
| tool        | after_delete   | ✓      |
| user        | before_create  | ✓      |
| user        | after_create   | ✓      |
| user        | before_update  | ✓      |
| user        | after_update   | ✓      |
| user        | before_delete  | ✓      |
| user        | after_delete   | ✓      |

## Verification

To verify the plugin is working correctly:

1. **Check Logs**: Look for hook invocation messages in stderr
2. **Test Rejection**: Configure reject mode and verify operations are blocked
3. **Test Modification**: Configure modify mode and verify objects are changed
4. **Test Metadata**: Configure metadata mode and verify metadata is added
5. **Test Priority**: Install multiple hook plugins and verify execution order

## Troubleshooting

**Plugin not loading:**
- Check that the plugin is enabled: `./mgw plugin list`
- Verify the binary path is correct and executable
- Check logs for plugin initialization errors

**Hooks not executing:**
- Verify plugin supports object_hooks: Check manifest.json
- Check hook registrations: Look for "Hook invoked" log messages
- Ensure AI Studio plugins are loaded on startup

**Configuration not applied:**
- Verify JSON is valid
- Check config schema validation
- Look for "Initialized with config" log message

## Development

This plugin demonstrates:
- Implementing all object hook types
- Configurable hook behavior
- Object modification
- Metadata storage
- Comprehensive logging
- Proper error handling
- Manifest and config schema embedding

Use this as a reference for developing your own object hook plugins.
