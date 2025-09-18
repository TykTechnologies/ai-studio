# Admin Token Setup Guide

This guide explains how to set up admin authentication for the microgateway management API.

## Authentication Architecture

The microgateway uses **two separate authentication systems**:

1. **Admin Authentication** - For management API endpoints (`/api/v1/*`)
2. **App Authentication** - For gateway proxy endpoints (`/llm/*`, `/tools/*`, `/datasource/*`)

## Admin Token Bootstrap

### Quick Setup

```bash
# 1. Build microgateway
make build-both

# 2. Set up environment
export ENCRYPTION_KEY="12345678901234567890123456789012"  # Exactly 32 chars
export JWT_SECRET="your-jwt-secret-key"

# 3. Run database migrations
./dist/microgateway -migrate

# 4. Create admin token
./dist/microgateway -create-admin-token

# 5. Save the token and use CLI
export MGW_TOKEN="<generated-admin-token>"
./dist/mgw system health
```

### Detailed Process

#### 1. Environment Setup
The microgateway requires two security keys:

```bash
# Generate secure keys
ENCRYPTION_KEY=$(openssl rand -hex 16)  # Exactly 32 hex characters
JWT_SECRET=$(openssl rand -hex 32)      # 64 hex characters

# Set environment
export ENCRYPTION_KEY="$ENCRYPTION_KEY"
export JWT_SECRET="$JWT_SECRET"

# Optional: Create .env file
cat > .env << EOF
ENCRYPTION_KEY=$ENCRYPTION_KEY
JWT_SECRET=$JWT_SECRET
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/microgateway.db?cache=shared&mode=rwc
LOG_LEVEL=info
EOF
```

#### 2. Database Initialization
```bash
# Create database schema
./dist/microgateway -migrate

# Verify migration success
echo "✅ Database migrations completed"
```

#### 3. Admin Token Creation
```bash
# Create long-lived admin token (30 days)
./dist/microgateway -create-admin-token \
  -admin-name="Production Admin" \
  -admin-expires="720h"

# Create temporary admin token (1 day)
./dist/microgateway -create-admin-token \
  -admin-name="Temporary Admin" \
  -admin-expires="24h"

# Create permanent admin token (no expiration)
./dist/microgateway -create-admin-token \
  -admin-name="Permanent Admin" \
  -admin-expires="0h"
```

**Example Output:**
```
✅ Admin token created successfully!
Token: 1a2b3c4d5e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0t1u2v3w4x5y6z7a8b9c0d1e2f
Name: Production Admin
Expires: 720h

Save this token - it won't be shown again!
Use it with the CLI: export MGW_TOKEN="1a2b3c4d5e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0t1u2v3w4x5y6z7a8b9c0d1e2f"
```

#### 4. CLI Configuration
```bash
# Set CLI environment
export MGW_URL="http://localhost:8080"
export MGW_TOKEN="<your-admin-token>"

# Test CLI access
./dist/mgw system health
./dist/mgw llm list

# Or create CLI config file
mkdir -p ~/.mgw
cat > ~/.mgw/config.yaml << EOF
url: http://localhost:8080
token: <your-admin-token>
format: table
verbose: false
EOF
```

## Admin Token Management

### Token Information
```bash
# Validate admin token
./dist/mgw token validate <admin-token>

# Get token information  
./dist/mgw token info <admin-token>

# List all admin tokens
./dist/mgw token list --app-id=1
```

### Token Rotation
```bash
# 1. Create new admin token
NEW_TOKEN=$(./dist/microgateway -create-admin-token \
  -admin-name="Rotated Admin Token" \
  -admin-expires="720h" | grep "Token:" | cut -d' ' -f2)

# 2. Test new token
export MGW_TOKEN="$NEW_TOKEN"
./dist/mgw system health

# 3. Revoke old token  
./dist/mgw token revoke <old-admin-token>

echo "✅ Admin token rotated successfully"
```

### Multiple Admin Tokens
```bash
# Create multiple admin tokens for different purposes
./dist/microgateway -create-admin-token -admin-name="Daily Operations" -admin-expires="24h"
./dist/microgateway -create-admin-token -admin-name="Weekly Maintenance" -admin-expires="168h"
./dist/microgateway -create-admin-token -admin-name="Emergency Access" -admin-expires="1h"
```

## Security Best Practices

### Secure Token Storage
```bash
# Store in secure location
echo "MGW_TOKEN=<admin-token>" >> ~/.env
chmod 600 ~/.env

# Use with systemd service
cat > /etc/systemd/system/mgw-admin.env << EOF
MGW_TOKEN=<admin-token>
MGW_URL=http://localhost:8080
EOF
chmod 600 /etc/systemd/system/mgw-admin.env
```

### Production Security
```bash
# Production environment setup
export ENCRYPTION_KEY=$(openssl rand -hex 16)
export JWT_SECRET=$(openssl rand -hex 32)

# Store in secrets management
kubectl create secret generic microgateway-admin \
  --from-literal=admin-token="<generated-admin-token>"

# Use with kubectl
kubectl get secret microgateway-admin -o jsonpath='{.data.admin-token}' | base64 -d
```

### Token Expiration Management
```bash
# Check token expiration
./dist/mgw token info <admin-token> | grep expires_at

# Set up token rotation cron job
cat > /etc/cron.d/mgw-token-rotation << 'EOF'
# Rotate admin token weekly (Sunday 2 AM)
0 2 * * 0 mgw-admin /path/to/rotate-admin-token.sh
EOF
```

## Troubleshooting

### Common Issues

#### Invalid Encryption Key
```bash
# Error: encryption key must be exactly 32 characters
# Solution: Use exactly 32-character key
export ENCRYPTION_KEY="12345678901234567890123456789012"
```

#### Database Connection Errors
```bash
# Error: Failed to connect to database
# Check database configuration
echo $DATABASE_DSN
./dist/microgateway -migrate  # Test database access
```

#### Token Generation Fails
```bash
# Error: Failed to generate admin token
# Check service container initialization
./dist/microgateway -migrate  # Ensure migrations ran
# Check database permissions
```

#### CLI Authentication Fails
```bash
# Error: authentication failed
# Verify token is correct
echo $MGW_TOKEN
./dist/mgw token validate $MGW_TOKEN

# Check microgateway is running
./dist/mgw system health
```

### Recovery Procedures

#### Lost Admin Token
```bash
# Generate new admin token (requires database access)
./dist/microgateway -create-admin-token -admin-name="Recovery Token"

# Use new token
export MGW_TOKEN="<new-admin-token>"
```

#### Corrupted Admin App
```bash
# The admin app (ID=1) will be recreated automatically
# when you run -create-admin-token
./dist/microgateway -create-admin-token
```

#### Database Reset
```bash
# Complete reset (DESTRUCTIVE!)
rm -f data/microgateway.db  # SQLite only
./dist/microgateway -migrate
./dist/microgateway -create-admin-token
```

## Production Deployment

### Initial Production Setup
```bash
# 1. Deploy microgateway to production
# 2. Run migrations in production environment  
./dist/microgateway -migrate

# 3. Create admin token on production server
./dist/microgateway -create-admin-token \
  -admin-name="Production Admin" \
  -admin-expires="720h"

# 4. Store token securely
# Save token in secrets management system
# Never commit tokens to version control
```

### Admin Access Control
```bash
# Create role-specific admin tokens
./dist/microgateway -create-admin-token -admin-name="Operations Team" -admin-expires="168h"
./dist/microgateway -create-admin-token -admin-name="Development Team" -admin-expires="72h"
./dist/microgateway -create-admin-token -admin-name="Emergency Access" -admin-expires="8h"
```

### Monitoring Admin Access
```bash
# Monitor admin token usage
./dist/mgw analytics events 1 --format=json | \
  jq '.data[] | select(.endpoint | contains("/api/v1/"))'

# List all admin tokens
./dist/mgw token list --app-id=1

# Check admin token expiration
./dist/mgw token list --app-id=1 --format=json | \
  jq '.data[] | {name, expires_at, last_used_at}'
```

This admin token system provides secure, database-access-controlled admin authentication that's completely separate from tenant app authentication.