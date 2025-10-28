# Custom Auth with UI - Hybrid Plugin Example

A complete example of a **hybrid plugin** that implements both `auth` and `studio_ui` hooks, demonstrating the universal plugin architecture in AI Studio.

## Overview

This plugin demonstrates how a single plugin can provide multiple capabilities:

- **Authentication Hook**: Custom token-based authentication with app and user mapping
- **Studio UI Hook**: Live web-based management interface for tokens

This is a production-ready example showing:
- Full auth implementation with token validation
- Complete UI with CRUD operations for token management
- RPC communication between UI and plugin backend
- Thread-safe token storage with mutex protection
- Token masking for security in the UI
- Embedded assets using Go's embed directive

## Features

### Authentication
- Custom token-based authentication
- Map tokens to specific App IDs and User IDs
- Optional token descriptions for management
- Bearer token support (automatically strips "Bearer " prefix)
- Configurable behavior for unknown tokens (reject or accept with default app ID)
- Thread-safe token operations

### UI Management
- Live token management interface accessible from AI Studio sidebar
- Add new tokens with app/user mappings
- Edit existing tokens (App ID, User ID, description)
- Delete tokens with confirmation
- Token masking for security (shows only first 4 and last 4 characters)
- Real-time statistics
- Responsive, modern design

## Directory Structure

```
custom-auth-ui/
├── server/
│   ├── main.go                 # Plugin implementation
│   ├── manifest.json           # Plugin manifest (both hooks declared)
│   ├── config.schema.json      # Configuration JSON schema
│   ├── ui/
│   │   ├── webc/
│   │   │   └── manager.js      # Web component for token management
│   │   └── assets/
│   │       └── shield.svg      # Sidebar icon
│   └── go.mod                  # Go module dependencies
└── README.md                   # This file
```

## Building the Plugin

### Prerequisites
- Go 1.21 or later
- AI Studio (Midsommar) installed

### Build Commands

```bash
cd examples/plugins/custom-auth-ui/server

# Initialize Go module (first time only)
go mod init github.com/TykTechnologies/midsommar/examples/plugins/custom-auth-ui/server
go mod tidy

# Build the plugin
go build -o custom-auth-ui main.go
```

## Configuration

### JSON Configuration Format

```json
{
  "tokens": [
    {
      "id": "token-1",
      "token": "my-secret-token-123",
      "app_id": 1,
      "user_id": "user123",
      "description": "Production API token"
    },
    {
      "token": "another-token-456",
      "app_id": 2,
      "user_id": "admin",
      "description": "Admin access token"
    }
  ],
  "reject_unknown_tokens": true,
  "default_app_id": 1
}
```

### Configuration Fields

- **tokens** (array): List of token configurations
  - **id** (string, optional): Unique identifier (auto-generated if not provided)
  - **token** (string, required): The authentication token value
  - **app_id** (integer, required): App ID this token authenticates (must be > 0)
  - **user_id** (string, optional): User identifier associated with this token
  - **description** (string, optional): Human-readable description

- **reject_unknown_tokens** (boolean, default: true):
  - `true`: Reject tokens not in the configured list
  - `false`: Accept unknown tokens with default_app_id

- **default_app_id** (integer, default: 1): Fallback App ID for unknown tokens (when reject_unknown_tokens is false)

### Minimal Configuration

You can start with an empty token list and manage tokens entirely via the UI:

```json
{
  "tokens": [],
  "reject_unknown_tokens": true
}
```

## Using the Plugin

### 1. Install the Plugin

Copy the built binary to your AI Studio plugins directory:

```bash
cp custom-auth-ui /path/to/midsommar/plugins/
```

### 2. Configure in AI Studio

Create a plugin configuration with your desired settings (or start with empty tokens array).

### 3. Access the UI

Once the plugin is active:
1. Look for "Custom Auth" in the AI Studio sidebar
2. Click "Token Management" to open the management interface
3. Use the UI to add, edit, or delete tokens

### 4. Authenticate Requests

Use your configured tokens in API requests:

```bash
curl -H "Authorization: Bearer my-secret-token-123" \
  https://your-ai-studio/api/v1/chat
```

The plugin will:
1. Extract the token value
2. Look it up in the configured tokens
3. Return the associated App ID and User ID
4. Reject unknown tokens (if reject_unknown_tokens is true)

## RPC Methods

The plugin exposes these RPC methods for the UI:

### `listTokens`
Returns all configured tokens with masked token values.

**Request**: `{}`

**Response**:
```json
{
  "tokens": [
    {
      "id": "token-1",
      "token_mask": "my-s********-123",
      "app_id": 1,
      "user_id": "user123",
      "description": "Production API token"
    }
  ],
  "count": 1
}
```

### `getToken`
Get details of a specific token by ID.

**Request**:
```json
{
  "id": "token-1"
}
```

**Response**: Token object with masked token value

### `addToken`
Add a new token.

**Request**:
```json
{
  "token": "new-token-789",
  "app_id": 1,
  "user_id": "newuser",
  "description": "New token"
}
```

**Response**:
```json
{
  "success": true,
  "id": "token-3",
  "message": "Token added successfully"
}
```

### `updateToken`
Update an existing token (cannot change token value, only metadata).

**Request**:
```json
{
  "id": "token-1",
  "app_id": 2,
  "user_id": "updated-user",
  "description": "Updated description"
}
```

**Response**:
```json
{
  "success": true,
  "message": "Token updated successfully"
}
```

### `deleteToken`
Delete a token.

**Request**:
```json
{
  "id": "token-1"
}
```

**Response**:
```json
{
  "success": true,
  "message": "Token deleted successfully"
}
```

## Implementation Details

### Auth Hook
- Implements `Authenticate()` and `ValidateToken()` methods from the plugin SDK
- Thread-safe token storage using `sync.RWMutex`
- Handles both raw tokens and Bearer format
- Returns structured auth responses with metadata

### Studio UI Hook
- Implements `GetManifest()`, `GetAsset()`, and `Call()` methods
- Assets embedded using `//go:embed` directive
- Serves JavaScript, SVG, and JSON assets
- RPC methods provide CRUD operations for token management

### Security Considerations
- Token values are masked in the UI (shows only first 4 and last 4 characters)
- Token values cannot be retrieved after creation (write-only)
- Token updates do not allow changing the token value itself
- Delete operations require confirmation
- All RPC methods validate input parameters

## Key Design Patterns

### 1. Hybrid Plugin Architecture
The plugin declares both hooks in the manifest:
```json
{
  "capabilities": {
    "hooks": ["auth", "studio_ui"],
    "primary_hook": "auth"
  }
}
```

### 2. Embedded Assets
All UI assets are embedded in the binary using Go's embed directive:
```go
//go:embed ui assets manifest.json
var embeddedAssets embed.FS
```

### 3. Thread-Safe Operations
All token operations are protected with read/write mutex:
```go
p.mu.RLock()
defer p.mu.RUnlock()
// ... read operations

p.mu.Lock()
defer p.mu.Unlock()
// ... write operations
```

### 4. Web Component UI
The UI is built as a Web Component (Custom Element):
```javascript
class CustomAuthManager extends HTMLElement {
  // ... component implementation
}
customElements.define('custom-auth-manager', CustomAuthManager);
```

### 5. RPC Communication
UI communicates with plugin backend via RPC:
```javascript
const result = await this.pluginAPI.call('listTokens', {});
```

## Testing

### Test Authentication

```bash
# Test with valid token
curl -H "Authorization: Bearer my-secret-token-123" \
  https://your-ai-studio/api/v1/chat

# Test with invalid token (should be rejected if reject_unknown_tokens is true)
curl -H "Authorization: Bearer invalid-token" \
  https://your-ai-studio/api/v1/chat
```

### Test UI
1. Open AI Studio in your browser
2. Navigate to "Custom Auth" → "Token Management"
3. Try adding, editing, and deleting tokens
4. Verify that tokens work for authentication after adding them

## Troubleshooting

### Plugin doesn't load
- Check that the binary is in the correct plugins directory
- Verify the manifest.json is properly formatted
- Check AI Studio logs for initialization errors

### UI doesn't appear
- Ensure the plugin is activated in AI Studio
- Check browser console for JavaScript errors
- Verify that assets are properly embedded in the binary

### Authentication fails
- Check that the token is in the configured list
- Verify the token format (with or without "Bearer " prefix)
- Check that the associated App ID exists in the database
- Review plugin logs for authentication attempts

## License

This example is part of the Midsommar AI Studio project and follows the same license.

## Learn More

- [AI Studio Documentation](https://github.com/TykTechnologies/midsommar)
- [Plugin SDK Reference](https://github.com/TykTechnologies/midsommar/tree/main/examples/plugins/sdk)
- [Other Plugin Examples](https://github.com/TykTechnologies/midsommar/tree/main/examples/plugins)
