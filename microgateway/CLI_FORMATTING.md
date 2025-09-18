# CLI Formatting Guide

The microgateway CLI provides intelligent table formatting optimized for terminal viewing with compact views showing only essential information.

## Compact Table Views

By default, the CLI shows **compact tables** with only the most important columns for each resource type:

### LLM List View
```bash
./dist/mgw llm list
```
**Compact Columns:** ID, NAME, VENDOR, MODEL, ACTIVE, BUDGET

Example output:
```
ID  NAME              VENDOR     MODEL                      ACTIVE  BUDGET
1   GPT-4 Production  openai     gpt-4                      ✅      $1000
2   Claude Sonnet 4   anthropic  claude-sonnet-4-20250514  ✅      unlimited
3   Local Llama       ollama     llama3.1:8b               ❌      unlimited
```

### App List View  
```bash
./dist/mgw app list
```
**Compact Columns:** ID, NAME, OWNER, BUDGET, ACTIVE

Example output:
```
ID  NAME         OWNER                BUDGET     ACTIVE
1   Admin System admin@microgateway.local  unlimited  ✅
2   My AI App    developer@company.com     $500       ✅
3   Test App     test@company.com          $100       ❌
```

### Credential List View
```bash
./dist/mgw credential list 2
```
**Compact Columns:** ID, NAME, KEY_ID, ACTIVE, EXPIRES

Example output:
```
ID  NAME               KEY_ID      ACTIVE  EXPIRES
1   Production Key     key_abc123  ✅      never
2   Temporary Key      key_def456  ✅      2024-12-31
```

### Token List View
```bash
./dist/mgw token list --app-id=2
```
**Compact Columns:** ID, NAME, APP_ID, SCOPES, EXPIRES

Example output:
```
ID  NAME          APP_ID  SCOPES      EXPIRES
1   Admin Token   1       admin       never  
2   API Token     2       api         2024-12-31
3   Read Token    2       read        2024-11-30
```

### Budget Usage View
```bash
./dist/mgw budget usage 2
```
**Compact Columns:** APP_ID, USAGE, BUDGET, REMAINING, % USED

Example output:
```
APP_ID  USAGE    BUDGET   REMAINING  % USED
2       $45.75   $500.00  $454.25    9.2%
```

## Detailed Views

Use the `--detailed` flag to see all available information:

```bash
# Show all LLM fields
./dist/mgw llm list --detailed

# Show all app fields  
./dist/mgw app list --detailed
```

## Alternative Output Formats

### JSON Output (Machine Readable)
```bash
./dist/mgw llm list --format=json
```

### YAML Output (Configuration Friendly)
```bash
./dist/mgw llm list --format=yaml
```

## Visual Indicators

The CLI uses visual indicators for better readability:

- **✅** - Active/enabled status
- **❌** - Inactive/disabled status
- **$100** - Budget amounts formatted as currency
- **unlimited** - No budget/rate limit set
- **never** - No expiration date
- **2024-12-31** - Date formatting for expiration dates

## Terminal Optimization

### Responsive Columns
- Long text fields are automatically truncated
- Endpoint URLs truncated to 30 characters with "..."
- Names and IDs always fully visible
- Monetary amounts formatted for readability

### Color Support
The rodaine/table library provides automatic color support when available:
- Headers are automatically styled
- Tables adapt to terminal width
- Clean borders and spacing

## Usage Examples

### Quick Resource Overview
```bash
# Quick LLM overview - see ID for creating apps
./dist/mgw llm list

# Quick app overview - see which apps exist
./dist/mgw app list

# Check budget status
./dist/mgw budget usage 2
```

### Detailed Information
```bash
# Full LLM details when needed
./dist/mgw llm list --detailed --format=yaml > llm-backup.yaml

# Full app configuration
./dist/mgw app get 2 --format=json

# Complete token information
./dist/mgw token list --app-id=2 --detailed
```

### Workflow Examples
```bash
# 1. Find LLM ID to use in app creation
./dist/mgw llm list
# Shows: ID=1 for "GPT-4 Production"

# 2. Create app with specific LLM
./dist/mgw app create --name="My App" --email=me@company.com --llm-ids="1"

# 3. Get the new app ID
./dist/mgw app list
# Shows: ID=3 for "My App"

# 4. Create credentials for the app
./dist/mgw credential create 3 --name="Production Key"
```

## Benefits

### Before (Original Formatting)
- **Wide tables** that don't fit in terminal windows
- **All columns shown** whether needed or not
- **No visual indicators** for status
- **Raw JSON field names** as headers
- **Difficult to scan** for specific information

### After (Improved Formatting)
- **✅ Compact views** showing only essential columns
- **✅ Terminal-friendly** width and spacing
- **✅ Visual indicators** (✅❌) for quick status scanning  
- **✅ Smart field selection** per resource type
- **✅ ID prominently displayed** for easy reference
- **✅ Professional appearance** with clean borders
- **✅ Detailed option** when full information is needed

The improved formatting makes the CLI much more practical for daily terminal use while still providing access to complete information when needed.