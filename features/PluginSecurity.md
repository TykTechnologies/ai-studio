# Plugin Security

**⚠️ ADVANCED FEATURES AVAILABLE IN ENTERPRISE EDITION ONLY**

Plugin security provides multiple layers of protection against malicious or misconfigured plugins. The system implements a **4-tier security model** with basic features in Community Edition and advanced features in Enterprise Edition.

---

## Table of Contents

- [Security Features Overview](#security-features-overview)
- [Community Edition Features](#community-edition-features)
- [Enterprise Edition Features](#enterprise-edition-features)
- [Architecture](#architecture)
- [Security Checks](#security-checks)
- [Configuration](#configuration)
- [API Reference](#api-reference)

---

## Security Features Overview

### Security Tiers

```
┌─────────────────────────────────────────────────────────────┐
│ Tier 1: Filesystem Security (CE + ENT)                      │
│ • Path Whitelisting                                          │
│ • Symlink Blocking                                           │
│ • Executable Validation                                      │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ Tier 2: Integrity Security (CE + ENT)                       │
│ • Checksum Validation (SHA256)                               │
│ • File Modification Detection                                │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ Tier 3: Network Security (ENT ONLY)                         │
│ • GRPC Host Whitelisting                                     │
│ • Internal IP Blocking (10.x, 192.168.x, 127.x, ::1)        │
│ • SSRF Prevention                                            │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ Tier 4: Supply Chain Security (ENT ONLY)                    │
│ • OCI Manifest Signature Verification (Cosign)              │
│ • Keyless Signing Support                                    │
│ • Policy-Based Verification                                  │
│ • Multiple Public Key Support                                │
└─────────────────────────────────────────────────────────────┘
```

---

## Community Edition Features

### 1. Path Whitelisting (Tier 1 - Filesystem Security)

**Purpose**: Prevents plugins from loading executables from dangerous locations

**Implementation**: [microgateway/plugins/manager.go:784-865](../microgateway/plugins/manager.go#L784-L865)

**Default Allowed Directories**:
```go
/opt/microgateway/plugins
./plugins
plugins/
```

**Security Checks**:
- ✅ Shell metacharacter detection (`; | & $( ) { } [ ] < > ? * ! ~`)
- ✅ Path traversal prevention (`../`, `..\\`)
- ✅ Absolute path resolution
- ✅ Symlink blocking
- ✅ Executable permission verification

**Example**:
```bash
# ✅ ALLOWED
file://./plugins/my-plugin
file:///opt/microgateway/plugins/validator

# ❌ BLOCKED
file://../../../etc/passwd
file:///usr/local/bin/malicious  # Outside allowed dirs
file://$(whoami)                  # Shell metacharacters
```

**Development Bypass**:
```bash
MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS=1  # Disables path restrictions
```

---

### 2. Checksum Validation (Tier 2 - Integrity Security)

**Purpose**: Ensures plugin files haven't been modified after deployment

**Implementation**:
- AI Studio: [services/plugin_service.go](../services/plugin_service.go)
- Microgateway: [microgateway/internal/services/plugin_service.go](../microgateway/internal/services/plugin_service.go#L307-L337)

**Database Schema**:
```sql
plugins (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  command TEXT NOT NULL,
  checksum TEXT,  -- SHA256 hash (optional)
  ...
)
```

**How It Works**:
1. Calculate SHA256 hash during plugin creation
2. Store hash in database
3. Validate hash before plugin loading (optional)
4. Detect file modifications

**Example**:
```go
// Calculate checksum
file, _ := os.Open(filePath)
hasher := sha256.New()
io.Copy(hasher, file)
calculatedChecksum := hex.EncodeToString(hasher.Sum(nil))

// Validate
if calculatedChecksum != plugin.Checksum {
    return fmt.Errorf("checksum mismatch: file has been modified")
}
```

**Tests**: [microgateway/internal/services/plugin_service_test.go:560-621](../microgateway/internal/services/plugin_service_test.go#L560-L621)

---

## Enterprise Edition Features

### 3. GRPC Host Whitelisting (Tier 3 - Network Security)

**⚠️ ENTERPRISE EDITION ONLY**

**Purpose**: Prevents plugins from targeting internal network addresses (SSRF protection)

**Implementation**: [enterprise/features/plugin_security/grpc_validator.go](../enterprise/features/plugin_security/grpc_validator.go)

**Blocked IP Ranges**:
```
10.0.0.0/8          Private Class A (10.x.x.x)
172.16.0.0/12       Private Class B (172.16-31.x.x)
192.168.0.0/16      Private Class C (192.168.x.x)
127.0.0.0/8         IPv4 loopback (127.x.x.x)
169.254.0.0/16      IPv4 link-local
::1/128             IPv6 loopback
fc00::/7            IPv6 unique local
fe80::/10           IPv6 link-local
```

**Blocked Hostnames**:
- `localhost`
- `::1`
- Any hostname containing "localhost"

**Example**:
```bash
# ✅ ALLOWED (public addresses)
grpc://plugins.company.com:50051
grpc://35.123.45.67:8080

# ❌ BLOCKED (internal addresses)
grpc://localhost:50051
grpc://127.0.0.1:8080
grpc://192.168.1.100:50051
grpc://10.0.0.5:8080
grpc://::1:50051
```

**Development Bypass**:
```bash
ALLOW_INTERNAL_NETWORK_ACCESS=true  # Allows internal IPs (development only)
```

**Community Edition Behavior**:
- ⚠️ All hosts allowed (no blocking)
- Logs one-time warning: "Plugin Security: GRPC host whitelisting is disabled in Community Edition"

---

### 4. OCI Signature Verification (Tier 4 - Supply Chain Security)

**⚠️ ENTERPRISE EDITION ONLY**

**Purpose**: Verifies plugin authenticity using cryptographic signatures (Cosign)

**Implementation**: [enterprise/features/plugin_security/signature_verifier.go](../enterprise/features/plugin_security/signature_verifier.go)

**Verification Methods**:

#### A. Key-Based Verification
```bash
# Sign plugin during build
cosign sign --key cosign.key registry/plugin:v1.0.0

# Verify during fetch (ENT automatic)
OCI_PLUGINS_PUBKEY_1="-----BEGIN PUBLIC KEY-----..."
OCI_PLUGINS_REQUIRE_SIGNATURE=true
```

#### B. Keyless Verification (OIDC)
```go
// Verify using certificate identity
err := securityService.VerifyBundle(ctx, ref,
    "https://github.com/login/oauth",  // Issuer
    "user@company.com")                // Subject
```

#### C. Policy-Based Verification
```bash
# Create policy file
cat > policy.yaml <<EOF
apiVersion: policy.sigstore.dev/v1beta1
kind: ClusterImagePolicy
metadata:
  name: plugin-policy
spec:
  images:
  - glob: "registry.company.com/plugins/*"
  authorities:
  - keyless:
      url: https://fulcio.sigstore.dev
EOF

# Verify with policy
err := securityService.VerifyWithPolicy(ctx, ref, "policy.yaml")
```

**Public Key Resolution**:

The verifier supports multiple key reference formats:

```bash
# 1. Numbered keys
OCI_PLUGINS_PUBKEY_1="-----BEGIN PUBLIC KEY-----..."
# Reference: "1" or ""  (uses first key)

# 2. Named keys
OCI_PLUGINS_PUBKEY_CI="-----BEGIN PUBLIC KEY-----..."
OCI_PLUGINS_PUBKEY_PROD="-----BEGIN PUBLIC KEY-----..."
# Reference: "CI" or "PROD"

# 3. File-based keys
OCI_PLUGINS_PUBKEY_FILE_COSIGN=/etc/ai-studio/cosign.pub
# Reference: "file:/etc/ai-studio/cosign.pub"

# 4. Direct file paths
# Reference: "/path/to/key.pub"

# 5. Environment variable reference
# Reference: "env:OCI_PLUGINS_PUBKEY_CI"
```

**Community Edition Behavior**:
- ⚠️ Signature verification skipped (no-op)
- `RequireSignature` setting ignored
- Logs one-time warning: "Plugin Security: OCI signature verification is disabled in Community Edition"

---

## Architecture

### Service Layer Pattern

```
┌─────────────────────────────────────────────────────────────┐
│ services/plugin_security/                                    │
│ ┌─────────────┐  ┌──────────┐  ┌──────────┐                │
│ │ interface.go│  │ types.go │  │ errors.go│  (Public)       │
│ └─────────────┘  └──────────┘  └──────────┘                │
│ ┌─────────────┐  ┌───────────────────┐                      │
│ │ factory.go  │  │ community.go      │  (Public)            │
│ └─────────────┘  └───────────────────┘                      │
└─────────────────────────────────────────────────────────────┘
                          ↓
        ┌─────────────────┴─────────────────┐
        │                                    │
┌───────▼────────┐                 ┌────────▼────────┐
│ Community Ed   │                 │ Enterprise Ed   │
│ (No-op stub)   │                 │ (Full security) │
└────────────────┘                 └─────────────────┘
     ⚠️ Allows                          🔒 Enforces
     all ops                            all checks
```

### Enterprise Implementation

```
enterprise/features/plugin_security/
├── service.go              # Main service implementation
├── init.go                 # Factory registration
├── grpc_validator.go       # GRPC host whitelisting
├── signature_verifier.go   # OCI signature verification
└── errors.go               # Enterprise error types
```

### Integration Points

**AI Studio Control Plane**:
```
main.go
  ↓ Creates auth.Config with OCIConfig
api/api.go
  ↓ Initializes plugin_security.Service
  ↓ Passes to validation functions
api/validation.go
  ↓ Uses securityService.ValidateGRPCHost()
api/plugin_handlers.go
  ↓ Calls validatePluginCommand(cmd, securityService)
```

**Microgateway Data Plane**:
```
cmd/microgateway/main.go
  ↓ Creates config.Config with OCIPlugins
internal/services/container.go
  ↓ Initializes plugin_security.Service
  ↓ Calls pluginManager.SetSecurityService()
plugins/manager.go
  ↓ Sets securityService on ociClient
pkg/ociplugins/client.go
  ↓ Uses securityService.VerifySignature()
```

---

## Security Checks

### Validation Flow

```
Plugin Command (e.g., "grpc://localhost:50051")
          ↓
┌─────────────────────────────────────────────┐
│ API Layer Validation                        │
│ (api/validation.go)                         │
│ - Command length check                      │
│ - Path traversal detection                  │
│ - Command injection detection               │
│ - URL parsing                                │
│ - Scheme validation                         │
│ - GRPC host validation (uses service) ← ENT │
└─────────────────────────────────────────────┘
          ↓
┌─────────────────────────────────────────────┐
│ Service Layer Validation                    │
│ (services/plugin_service.go)                │
│ - Plugin conflict check                     │
│ - Database validation                       │
└─────────────────────────────────────────────┘
          ↓
┌─────────────────────────────────────────────┐
│ Manager Layer Security                      │
│ (plugins/manager.go)                        │
│ - Path whitelisting ← CE                    │
│ - Checksum validation ← CE                  │
│ - OCI signature verification ← ENT          │
└─────────────────────────────────────────────┘
```

### Security Decision Matrix

| Plugin Command | CE Security | ENT Security |
|---------------|-------------|--------------|
| `file://./plugins/my-plugin` | ✅ Path check<br>✅ Checksum | ✅ Path check<br>✅ Checksum |
| `grpc://localhost:50051` | ⚠️ Allowed | ❌ Blocked (internal IP) |
| `grpc://plugins.company.com:50051` | ✅ Allowed | ✅ Allowed (public) |
| `oci://registry/plugin:v1` (unsigned) | ✅ Downloaded | ❌ Blocked (no signature) |
| `oci://registry/plugin:v1` (signed) | ✅ Downloaded | ✅ Verified + Downloaded |

---

## Community Edition Features

### Path Whitelisting

**File**: [microgateway/plugins/manager.go:784-865](../microgateway/plugins/manager.go#L784-L865)

**Validations Performed**:
1. Shell metacharacter detection
2. Path traversal prevention
3. Directory whitelist enforcement
4. Symlink rejection
5. Executable permission check

**Code**:
```go
func validatePluginPath(cmdPath string) error {
    // 1. Check for shell metacharacters
    if shellMetacharPattern.MatchString(cmdPath) {
        return fmt.Errorf("🔒 SECURITY: Plugin command contains shell metacharacters")
    }

    // 2. Resolve to absolute path
    absPath, err := filepath.Abs(cmdPath)

    // 3. Check if path is within allowed directories
    if !isPathInAllowedDirectories(absPath) {
        return fmt.Errorf("🔒 SECURITY: Plugin path not in allowed directories")
    }

    // 4. Reject symbolic links
    if fileInfo.Mode()&os.ModeSymlink != 0 {
        return fmt.Errorf("🔒 SECURITY: Plugin cannot be a symbolic link")
    }

    // 5. Verify file is executable
    if fileInfo.Mode()&0111 == 0 {
        return fmt.Errorf("🔒 SECURITY: Plugin file is not executable")
    }
}
```

---

### Checksum Validation

**File**: [microgateway/internal/services/plugin_service.go:307-337](../microgateway/internal/services/plugin_service.go#L307-L337)

**Database Field**:
```go
type Plugin struct {
    Checksum string `json:"checksum"` // SHA256 hash
}
```

**Usage**:
```go
// Calculate checksum during creation
hasher := sha256.New()
io.Copy(hasher, file)
plugin.Checksum = hex.EncodeToString(hasher.Sum(nil))

// Validate before loading
if err := pluginService.ValidatePluginChecksum(pluginID, filePath); err != nil {
    return fmt.Errorf("plugin file has been modified: %w", err)
}
```

**Tests**: [microgateway/internal/services/plugin_service_test.go:560-621](../microgateway/internal/services/plugin_service_test.go#L560-L621)

---

## Enterprise Edition Features

### GRPC Host Whitelisting

**File**: [enterprise/features/plugin_security/grpc_validator.go](../enterprise/features/plugin_security/grpc_validator.go)

**CIDR Range Detection**:
```go
func (v *GRPCValidator) isInternalIP(host string) bool {
    privateCIDRs := []string{
        "10.0.0.0/8",        // Private Class A
        "172.16.0.0/12",     // Private Class B
        "192.168.0.0/16",    // Private Class C
        "127.0.0.0/8",       // IPv4 loopback
        "169.254.0.0/16",    // IPv4 link-local
        "::1/128",           // IPv6 loopback
        "fc00::/7",          // IPv6 unique local
        "fe80::/10",         // IPv6 link-local
    }

    // Check if IP falls within any private CIDR range
    for _, cidrStr := range privateCIDRs {
        _, cidr, _ := net.ParseCIDR(cidrStr)
        if cidr.Contains(ip) {
            return true
        }
    }
    return false
}
```

**Attack Prevention**:
- **SSRF (Server-Side Request Forgery)**: Prevents malicious plugins from accessing internal services
- **Cloud Metadata Attacks**: Blocks access to cloud provider metadata endpoints (169.254.169.254)
- **Internal Service Discovery**: Prevents scanning of internal networks

**Error Messages**:
```
CE: No error (allowed)
ENT: "🔒 SECURITY: plugin command targets internal network address: grpc://127.0.0.1:50051"
```

---

### OCI Signature Verification

**File**: [enterprise/features/plugin_security/signature_verifier.go](../enterprise/features/plugin_security/signature_verifier.go)

**Dependencies**:
- **Cosign CLI**: Sigstore signing tool (must be installed)
- **Public Keys**: PEM-formatted public keys for verification

**Verification Process**:
```go
func (v *SignatureVerifier) Verify(ctx, ref, pubKeyID) error {
    // 1. Resolve public key reference
    pubKeyPath, err := v.getPublicKeyPath(pubKeyID)

    // 2. Build cosign verify command
    cmd := exec.CommandContext(ctx, "cosign", "verify",
        "--key", pubKeyPath,
        ref.FullReference())

    // 3. Run verification
    output, err := cmd.CombinedOutput()
    if err != nil {
        return ErrSignatureVerificationFailed
    }

    return nil
}
```

**Public Key Resolution Examples**:
```go
// Numeric reference → OCI_PLUGINS_PUBKEY_1
pubKeyPath, err := verifier.getPublicKeyPath("1")

// Named reference → OCI_PLUGINS_PUBKEY_CI
pubKeyPath, err := verifier.getPublicKeyPath("CI")

// Direct file path
pubKeyPath, err := verifier.getPublicKeyPath("/etc/keys/cosign.pub")

// File reference
pubKeyPath, err := verifier.getPublicKeyPath("file:/etc/keys/cosign.pub")

// Environment variable
pubKeyPath, err := verifier.getPublicKeyPath("env:MY_PUBKEY")
```

**Advanced Features**:

**Keyless Signing** (OIDC-based):
```go
// No pre-shared key required
err := securityService.VerifyBundle(ctx, ref,
    "https://token.actions.githubusercontent.com",  // Issuer
    "https://github.com/myorg/repo/.github/workflows/build.yml@refs/heads/main")  // Subject
```

**Policy-Based Verification**:
```yaml
# policy.yaml
apiVersion: policy.sigstore.dev/v1beta1
kind: ClusterImagePolicy
metadata:
  name: require-keyless-signature
spec:
  images:
  - glob: "registry.company.com/plugins/*"
  authorities:
  - keyless:
      url: https://fulcio.sigstore.dev
      identities:
      - issuer: https://token.actions.githubusercontent.com
        subject: "https://github.com/myorg/*"
```

**Error Messages**:
```
CE: No error (skipped)
ENT (failure): "signature verification failed: cosign verify failed: no signatures found"
ENT (success): Signature verification passed (logged at debug level)
```

---

## Configuration

### Environment Variables

**Community Edition**:
```bash
# Path whitelisting (CE)
MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS=0  # 0=enabled, 1=disabled (dev only)
```

**Enterprise Edition**:
```bash
# GRPC host whitelisting (ENT)
ALLOW_INTERNAL_NETWORK_ACCESS=false  # false=enforced, true=bypassed (dev only)

# OCI signature verification (ENT)
AI_STUDIO_OCI_REQUIRE_SIGNATURE=false     # AI Studio default (permissive)
OCI_PLUGINS_REQUIRE_SIGNATURE=true        # Microgateway default (strict)

# Public keys (ENT)
OCI_PLUGINS_PUBKEY_1="-----BEGIN PUBLIC KEY-----..."
OCI_PLUGINS_PUBKEY_CI="-----BEGIN PUBLIC KEY-----..."
OCI_PLUGINS_PUBKEY_FILE_COSIGN=/etc/keys/cosign.pub
```

### Code Configuration

**AI Studio** ([config/oci_config.go](../config/oci_config.go)):
```go
type OCIConfig struct {
    RequireSignature bool  `env:"AI_STUDIO_OCI_REQUIRE_SIGNATURE" envDefault:"false"`
    // ... other fields
}
```

**Microgateway** ([microgateway/internal/config/plugin_config.go](../microgateway/internal/config/plugin_config.go)):
```go
type OCIPluginConfig struct {
    RequireSignature bool  `env:"OCI_PLUGINS_REQUIRE_SIGNATURE" envDefault:"true"`
    // ... other fields
}
```

---

## API Reference

### Plugin Security Service Interface

**Location**: [services/plugin_security/interface.go](../services/plugin_security/interface.go)

```go
type Service interface {
    // GRPC host validation (ENT only)
    ValidateGRPCHost(host string) error
    IsInternalIP(host string) bool

    // Signature verification (ENT only)
    VerifySignature(ctx context.Context, ref *OCIReference, pubKeyID string) error
    VerifyBundle(ctx context.Context, ref *OCIReference, issuer, subject string) error
    VerifyWithPolicy(ctx context.Context, ref *OCIReference, policyPath string) error

    // Public key management (ENT only)
    GetPublicKeyPath(pubKeyID string) (string, error)
    ValidatePublicKey(keyPath string) error
    LoadPublicKeysFromDirectory(dir string) ([]string, error)
}
```

### Community Edition Implementation

**Location**: [services/plugin_security/community.go](../services/plugin_security/community.go)

**Behavior**: All methods return nil (allow) or empty values

```go
// ValidateGRPCHost always returns nil in CE
func (s *communityService) ValidateGRPCHost(host string) error {
    s.logSecurityWarning("GRPC host whitelisting")
    return nil  // Allow all hosts
}

// VerifySignature always returns nil in CE
func (s *communityService) VerifySignature(ctx, ref, pubKeyID) error {
    s.logSecurityWarning("OCI signature verification")
    return nil  // Skip verification
}
```

### Enterprise Edition Implementation

**Location**: `enterprise/features/plugin_security/service.go`

**Behavior**: Full security enforcement with proper validation

```go
// ValidateGRPCHost blocks internal IPs in ENT
func (s *enterpriseService) ValidateGRPCHost(host string) error {
    return s.grpcValidator.ValidateHost(host)  // Returns error if internal
}

// VerifySignature performs Cosign verification in ENT
func (s *enterpriseService) VerifySignature(ctx, ref, pubKeyID) error {
    return s.signatureVerifier.Verify(ctx, ref, pubKeyID)
}
```

---

## Edition Detection

**Check if enterprise security is available**:
```go
import "github.com/TykTechnologies/midsommar/v2/services/plugin_security"

if plugin_security.IsEnterpriseAvailable() {
    // Enterprise Edition
    log.Println("✅ Advanced plugin security enabled")
} else {
    // Community Edition
    log.Println("⚠️  Using basic plugin security - upgrade to Enterprise for advanced features")
}
```

---

## Security Best Practices

### Production Deployments

**Community Edition**:
1. ✅ Enable path whitelisting (default)
2. ✅ Enable checksum validation (recommended)
3. ⚠️  Understand GRPC host whitelisting is disabled
4. ⚠️  Understand OCI signature verification is disabled
5. 🔒 Consider upgrading to Enterprise for production

**Enterprise Edition**:
1. ✅ Enable all security features (defaults)
2. ✅ Configure public keys for OCI verification
3. ✅ Set `ALLOW_INTERNAL_NETWORK_ACCESS=false` (default)
4. ✅ Enable `RequireSignature=true` for OCI plugins
5. 🔒 Use policy-based verification for advanced control

### Development Environments

**Bypass Settings (Use with Caution)**:
```bash
# Bypass path restrictions (CE)
MICROGATEWAY_ALLOW_ALL_PLUGIN_PATHS=1

# Bypass GRPC host whitelisting (ENT)
ALLOW_INTERNAL_NETWORK_ACCESS=true

# Disable signature verification (ENT)
OCI_PLUGINS_REQUIRE_SIGNATURE=false
AI_STUDIO_OCI_REQUIRE_SIGNATURE=false
```

**⚠️ WARNING**: Never use bypass settings in production!

---

## Upgrade Path

### From CE to ENT

When upgrading from Community Edition to Enterprise Edition:

1. **No Database Migration Required** - Security settings are environment-based
2. **Configure Public Keys** - Set OCI_PLUGINS_PUBKEY_* variables
3. **Review Plugin Commands** - Ensure no internal IP targets
4. **Test Plugin Loading** - Verify signature verification works
5. **Update Monitoring** - Watch for security violation logs

### Backward Compatibility

- ✅ CE configurations work in ENT
- ✅ Plugin database models unchanged
- ✅ API endpoints remain consistent
- ✅ No breaking changes to plugin SDK

---

## Troubleshooting

### Common Issues

**CE: "Plugin Security: GRPC host whitelisting is disabled"**
- **Cause**: Using Community Edition
- **Solution**: Upgrade to Enterprise Edition or accept reduced security

**ENT: "plugin command targets internal network address"**
- **Cause**: Plugin trying to access internal IP
- **Solution**:
  - Use public IP/hostname instead
  - Set `ALLOW_INTERNAL_NETWORK_ACCESS=true` (development only)

**ENT: "signature verification failed: no signatures found"**
- **Cause**: Plugin image not signed
- **Solution**:
  - Sign plugin: `cosign sign --key cosign.key registry/plugin:v1`
  - Or disable verification: `OCI_PLUGINS_REQUIRE_SIGNATURE=false` (not recommended)

**ENT: "public key not found"**
- **Cause**: Public key reference not configured
- **Solution**: Set `OCI_PLUGINS_PUBKEY_1` or relevant key variable

---

## Related Documentation

- [Plugin System Overview](./README.md)
- [Plugin SDK](../pkg/plugin_sdk/README.md)
- [OCI Plugin Distribution](../microgateway/docs/extensibility/plugin-distribution.md)
- [Enterprise Framework](../ENTERPRISE_FRAMEWORK.md)

---

## Testing

### Community Edition Tests

**Location**: Tests are integrated into existing plugin tests

**Coverage**:
- ✅ Path whitelisting enforcement
- ✅ Checksum validation (valid, invalid, missing)
- ✅ Plugin loading with basic security

### Enterprise Edition Tests

**Location**: `enterprise/features/plugin_security/` (when implemented)

**Coverage**:
- ✅ GRPC host validation (internal IPs, public IPs, localhost)
- ✅ Signature verification (valid, invalid, keyless)
- ✅ Public key resolution (all formats)
- ✅ Policy-based verification
- ✅ Development bypass

---

## Summary

Plugin Security provides **defense in depth** with multiple security layers:

**Community Edition** provides **basic security** sufficient for development and low-risk deployments:
- Filesystem protection via path whitelisting
- File integrity via checksum validation

**Enterprise Edition** adds **advanced security** required for production deployments:
- Network protection via GRPC host whitelisting (SSRF prevention)
- Supply chain security via OCI signature verification (trusted sources)

The security model ensures that even CE users have fundamental protections, while ENT users get enterprise-grade security controls for production environments.
