# Branding Customization

## Overview

The branding customization system allows administrators to customize the visual appearance of the Tyk AI Portal without requiring code changes or application rebuilds. This feature enables white-labeling and brand consistency for organizations deploying the platform.

## Features

### System-Wide Customization
- **Scope**: All branding changes apply system-wide to all users
- **Access**: Admin-only functionality
- **Persistence**: Settings and assets (logo, favicon) stored in the database, so every replica serves them
- **Real-time**: Changes apply immediately on page refresh

### Customizable Elements

1. **Logo (Header)**
   - Replace the default Tyk logo in the header navigation
   - Supported formats: PNG, JPG, SVG
   - Maximum size: 2MB
   - Served via API endpoint: `/api/v1/branding/logo`

2. **Favicon (Browser Icon)**
   - Replace the browser tab icon
   - Supported formats: ICO, PNG
   - Maximum size: 100KB
   - Served via API endpoint: `/api/v1/branding/favicon`

3. **Color Scheme**
   - **Primary Color**: Main brand color (default: `#23E2C2`)
   - **Secondary Color**: Accent color (default: `#343452`)
   - **Background Color**: Default background (default: `#FFFFFF`)
   - All colors must be valid hex codes
   - Applied via Material-UI theme generation

4. **Application Title**
   - Browser tab title
   - Maximum length: 50 characters
   - Default: "Tyk AI Portal"

5. **Custom CSS**
   - Advanced feature for custom styling
   - Injected globally into the application
   - Use with caution - can affect application functionality

## Architecture

### Backend Components

#### Data Model
- **File**: `models/branding_settings.go`
- **Pattern**: Singleton (ID=1)
- **Fields**:
  - `logo_path`: Name of the custom logo (`logo.png`, `logo.jpg`, `logo.svg`); empty means the default. It says a custom logo is set; the bytes are in `branding_assets`
  - `favicon_path`: Name of the custom favicon (`favicon.ico`, `favicon.png`); empty means the default
  - `app_title`: Custom application title
  - `primary_color`: Hex color for primary theme
  - `secondary_color`: Hex color for secondary theme
  - `background_color`: Hex color for background
  - `custom_css`: Custom CSS rules

#### Asset Storage
- **Files**: `models/branding_asset.go`, `services/branding_assets.go`
- **Table**: `branding_assets`, one row per kind (`logo`, `favicon`): the bytes, the content type they are served with, their SHA-256 (`hash`, the ETag) and size. An upload replaces the row and sets the settings' `logo_path` / `favicon_path` in one transaction; reset deletes the rows and resets the settings in one transaction. Works the same on SQLite and Postgres.
- **Why the database**: assets were files under `BRANDING_STORAGE_PATH` on the replica that received the upload. Replicas in one site could share a volume; two sites cannot sensibly, so the other site served no logo (see `ActiveActive.md`, Phase 6).
- **Validation** (unchanged): the declared content type must be an allowed one and the declared size within the limit; in addition the bytes are read with the limit applied, so a file larger than it declares is refused too.
- **Serving**: `GET /api/v1/branding/logo` and `/favicon` answer from the database through `Service.BrandingLogo` / `BrandingFavicon`, with `ETag: "<hash>"` and `Cache-Control: no-cache` (the browser keeps the asset and revalidates; `304` when unchanged). With no stored asset they serve the embedded default, without those headers. An uploaded asset is also sent with `X-Content-Type-Options: nosniff` and `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox`, so an SVG logo opened directly (not in an `<img>`, where scripts never run) cannot run script on Studio's origin. A failure to read the asset answers `500` with no cause; the cause is logged.
- **In-process cache**: each replica keeps the two assets in memory (`brandingAssetCache` on the `Service`). An upload or reset clears the local copy and sends `replicas.SignalBranding`; other replicas clear theirs when it arrives (`pkg/studio` registers `InvalidateBrandingAssets`), so they serve the new asset within about a second. As a backstop for a lost signal, a copy older than 30 s is checked against the stored hash (one small query) before it is served. If the database cannot be read, a copy already held is served.
- **Import of files from before 2.3**: at start (`Service.ImportBrandingFiles`, called by `studio.New` once the replica has joined the cluster), an asset is copied from `BRANDING_STORAGE_PATH` into the database when the settings name a custom one, the database has none, and the file is on this replica's disk. That makes it happen once, on the first replica that has the file, and never after a reset (the settings then name none). The insert does nothing if another replica got there first. Files are left in place and are not read again. A name is reduced to its base name, the extension must be one the asset can have, and the size limit applies.
- **Legacy file code**: `services/branding_file_storage.go` (`BrandingFileStorage`, `GetLogoFilePath`, `GetFaviconFilePath`) is no longer used to store or serve; `GetBrandingStoragePath` / `SetBrandingStoragePath` locate the directory for the import.

#### Business Logic
- **File**: `services/branding_service.go`
- **Responsibilities**:
  - Admin authorization enforcement
  - Hex color validation
  - Settings CRUD operations
  - File upload coordination

#### API Endpoints
- **File**: `api/branding_handlers.go`
- **Public Endpoints**:
  - `GET /api/v1/branding/settings` - Get current branding settings
  - `GET /api/v1/branding/logo` - Serve custom logo (or default)
  - `GET /api/v1/branding/favicon` - Serve custom favicon (or default)

- **Admin Endpoints** (require authentication):
  - `PUT /api/v1/branding/settings` - Update branding settings
  - `POST /api/v1/branding/logo` - Upload custom logo
  - `POST /api/v1/branding/favicon` - Upload custom favicon
  - `POST /api/v1/branding/reset` - Reset all branding to defaults

### Frontend Components

#### Dynamic Theme Generation
- **File**: `ui/admin-frontend/src/admin/theme.js`
- **Function**: `generateTheme(brandingConfig)`
- Generates Material-UI theme based on branding colors
- Supports runtime theme changes without rebuild

#### Application Bootstrap
- **File**: `ui/admin-frontend/src/App.js`
- Loads branding config on application startup
- Applies:
  - Dynamic theme via `generateTheme()`
  - Custom CSS injection
  - Application title
  - Dynamic favicon

#### Logo Integration
- **File**: `ui/admin-frontend/src/components/common/TopNavigation.js`
- Logo served from API endpoint with fallback
- Automatic error handling for missing custom logos

#### Admin UI
- **File**: `ui/admin-frontend/src/admin/pages/BrandingSettings.js`
- Complete branding management interface
- Features:
  - Logo/favicon upload with preview
  - Color picker with live preview
  - App title editor
  - Custom CSS editor (collapsible advanced section)
  - Reset to defaults with confirmation
  - Form validation and error handling

#### API Service
- **File**: `ui/admin-frontend/src/admin/services/brandingService.js`
- Frontend API client for branding operations
- Handles multipart form data for file uploads
- Error handling via centralized error handler

### Navigation
- **Route**: `/admin/branding`
- **Location**: Governance section in admin sidebar
- **Access**: Admin users only

## Configuration

### Environment Variables

- `BRANDING_STORAGE_PATH`: Directory where versions before 2.3 stored the logo and favicon
  - Default: `./data/branding`
  - Read once at start to import those files into the database. Nothing is written there any more, and it needs no volume once the import has happened (the log says `Branding: imported the logo from ...`).

### Frontend Config

Branding settings are automatically included in the frontend config response at `/auth/config`:

```json
{
  "branding": {
    "app_title": "Tyk AI Portal",
    "primary_color": "#23E2C2",
    "secondary_color": "#343452",
    "background_color": "#FFFFFF",
    "custom_css": "",
    "has_custom_logo": false,
    "has_custom_favicon": false
  }
}
```

## Usage

### Admin Workflow

1. Navigate to **Settings > Branding** in the admin panel
2. Customize desired elements:
   - Upload logo and/or favicon
   - Adjust color scheme
   - Modify application title
   - Add custom CSS (optional)
3. Click **Save Changes**
4. Refresh page to see changes applied

### Resetting to Defaults

1. Navigate to **Settings > Branding**
2. Click **Reset to Defaults**
3. Confirm the action
4. All branding reverts to Tyk defaults
5. Refresh page to see changes

## Container Deployment

The logo and favicon are in the database: they survive container restarts and are served by every replica, with no volume to mount.

Upgrading from a version before 2.3 with a custom logo or favicon: start the first 2.3 replica with the old branding volume still mounted (and `BRANDING_STORAGE_PATH` unchanged if you set it), so the files are imported. After that the volume can go.

## File Size Limits

- **Logo**: 2MB maximum (enforced by backend)
- **Favicon**: 100KB maximum (enforced by backend)

Oversized files will be rejected with an appropriate error message.

## Browser Support

- Modern browsers with support for:
  - CSS custom properties
  - ES6 JavaScript
  - Material-UI v6 requirements

## Security Considerations

1. **Admin-Only Access**: All write operations require admin authentication
2. **File Validation**: MIME type and size validation prevents malicious uploads
3. **CSS Injection**: Custom CSS is admin-only and injected via `dangerouslySetInnerHTML`
   - Only trusted administrators should have access
   - CSS can affect application behavior
4. **Path Safety**: Uploads are not written to disk. The start-up import reads only a base file name inside `BRANDING_STORAGE_PATH`

## Performance

- **Asset Caching**: Logo and favicon endpoints send an ETag and answer `304` to a browser that has the current asset
- **In-memory copies**: each replica serves the assets from memory, checking the stored hash at most every 30 s
- **Theme Generation**: Computed once per page load
- **Custom CSS**: Minimal performance impact (loaded as inline style)

## Troubleshooting

### Logo/Favicon Not Appearing

1. Check file size limits (2MB logo, 100KB favicon)
2. Verify supported format (PNG/JPG/SVG for logo, ICO/PNG for favicon)
3. Clear browser cache
4. Check browser console for 404 errors

### Colors Not Applying

1. Verify hex color format (must start with `#` and have 6 hex digits)
2. Refresh page after saving
3. Check browser console for theme generation errors

### Custom CSS Not Working

1. Verify CSS syntax
2. Check for CSS specificity conflicts
3. Use browser DevTools to inspect applied styles
4. Consider using `!important` for overrides (use sparingly)

### Permission Errors

1. Verify user has admin role
2. Check authentication token validity
3. Review server logs for authorization errors

## Future Enhancements

Potential future improvements:
- Font customization (typeface, weights)
- Additional color options (error, warning, success)
- CSS preprocessor support (SASS/LESS)
- Theme preview before saving
- Multiple theme profiles
- Per-tenant branding (multi-tenancy support)
- Advanced logo positioning/sizing options
