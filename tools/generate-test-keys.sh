#!/bin/bash
# tools/generate-test-keys.sh
# Generates test cosign keypairs for OCI plugin development

set -e

KEYS_DIR="testdata/keys"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FULL_KEYS_DIR="$REPO_ROOT/$KEYS_DIR"

echo "🔐 Generating test cosign keypairs for OCI plugin development..."
echo "📁 Keys will be stored in: $FULL_KEYS_DIR"

# Create keys directory
mkdir -p "$FULL_KEYS_DIR"

# Check if cosign is available
if ! command -v cosign &> /dev/null; then
    echo "❌ Error: cosign command not found"
    echo "Please install cosign: https://docs.sigstore.dev/cosign/installation/"
    exit 1
fi

# Generate test keypair for CI/automated testing
echo "🔑 Generating test-plugin-ci keypair..."
cd "$FULL_KEYS_DIR"
cosign generate-key-pair --output-key-prefix test-plugin-ci << EOF
test-password
test-password
EOF

# Generate development keypair
echo "🔑 Generating dev-plugin keypair..."
cosign generate-key-pair --output-key-prefix dev-plugin << EOF
dev-password
dev-password
EOF

# Set appropriate permissions
chmod 600 *.key  # Private keys - restrictive permissions
chmod 644 *.pub  # Public keys - readable

echo ""
echo "✅ Test keypairs generated successfully!"
echo ""
echo "📋 Generated files:"
echo "   🔐 test-plugin-ci.key (private) - for CI/testing"
echo "   🔓 test-plugin-ci.pub (public) - for verification"
echo "   🔐 dev-plugin.key (private) - for development"
echo "   🔓 dev-plugin.pub (public) - for development"
echo ""
echo "🔧 To use in tests, set environment variables:"
echo 'export OCI_PLUGINS_PUBKEY_1="$(cat testdata/keys/test-plugin-ci.pub)"'
echo 'export OCI_PLUGINS_PUBKEY_DEV="$(cat testdata/keys/dev-plugin.pub)"'
echo ""
echo "⚠️  Note: These are TEST KEYS ONLY - never use in production!"
echo "🔄 Private key password for tests: 'test-password' and 'dev-password'"