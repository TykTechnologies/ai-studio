---
title: "Bare Metal / VM Installation"
weight: 3
---

# Bare Metal / VM Installation

This guide covers installing Tyk AI Studio and the optional Microgateway on Linux servers using DEB or RPM packages. This is suitable for bare-metal servers, virtual machines, and cloud instances.

## Prerequisites

- **OS**: Ubuntu/Debian (DEB) or RHEL/CentOS/Amazon Linux (RPM)
- **Architecture**: amd64 (x86_64), arm64 (aarch64), or s390x
- **PostgreSQL 14+** (for AI Studio production use; SQLite is the default for development)
- **systemd** (for service management)
- Root or sudo access

## Edition Selection

Packages are available in Community and Enterprise editions:

| Component | Community Package | Enterprise Package |
|-----------|-------------------|--------------------|
| AI Studio | `tyk-ai-studio` | `tyk-ai-studio-ee` |
| Microgateway | `tyk-microgateway` | `tyk-microgateway-ee` |

Community packages are published to component-specific repos. Enterprise packages for **both** components are published to a single shared repo: `tyk/tyk-ee-unstable`.

## Generate Secrets

Before configuring, generate the required secret keys:

```bash
# Secret key for encryption (used for secrets management and SSO)
openssl rand -hex 16

# Encryption key for microgateway communication (must be exactly 32 hex chars)
openssl rand -hex 16

# gRPC auth token (for hub-spoke communication)
openssl rand -hex 16
```

Save these values — you will need them for both the AI Studio and Microgateway configuration.

---

## Part 1: Install AI Studio

### Add Package Repository

**Debian / Ubuntu (DEB):**

```bash
# Community Edition
curl -s https://packagecloud.io/install/repositories/tyk/tyk-ai-studio/script.deb.sh | sudo bash

# Enterprise Edition
curl -s https://packagecloud.io/install/repositories/tyk/tyk-ee-unstable/script.deb.sh | sudo bash
```

**RHEL / CentOS / Amazon Linux (RPM):**

```bash
# Community Edition
curl -s https://packagecloud.io/install/repositories/tyk/tyk-ai-studio/script.rpm.sh | sudo bash

# Enterprise Edition
curl -s https://packagecloud.io/install/repositories/tyk/tyk-ee-unstable/script.rpm.sh | sudo bash
```

### Install the Package

**DEB:**

```bash
# Community Edition
sudo apt-get install tyk-ai-studio

# Enterprise Edition
sudo apt-get install tyk-ai-studio-ee
```

**RPM:**

```bash
# Community Edition
sudo yum install tyk-ai-studio

# Enterprise Edition
sudo yum install tyk-ai-studio-ee
```

The package installs:

| Path | Description |
|------|-------------|
| `/opt/tyk-ai-studio/tyk-ai-studio` | Application binary |
| `/opt/tyk-ai-studio/tyk-ai-studio.conf.example` | Example configuration |
| `/etc/default/tyk-ai-studio` | Environment configuration (systemd) |
| `/lib/systemd/system/tyk-ai-studio.service` | Systemd service unit |

The installer automatically creates a `tyk` user and group to run the service.

### Configure AI Studio

Edit the environment configuration file:

```bash
sudo nano /etc/default/tyk-ai-studio
```

At minimum, set these values:

```env
# Security — REQUIRED: replace with your generated secrets
TYK_AI_SECRET_KEY=your-generated-secret-key
MICROGATEWAY_ENCRYPTION_KEY=your-generated-encryption-key

# Site URL — set to your server's hostname/IP
SITE_URL=http://your-server:8080

# Admin email
ADMIN_EMAIL=admin@example.com

# Database — SQLite is default, use PostgreSQL for production
DATABASE_TYPE=postgres
DATABASE_URL=postgresql://user:password@localhost:5432/tyk_ai_studio?sslmode=require
```

For hub-spoke deployments with a Microgateway, also set:

```env
# Hub-Spoke: Control Plane Mode
GATEWAY_MODE=control
GRPC_PORT=50051
GRPC_HOST=0.0.0.0
GRPC_TLS_INSECURE=true
GRPC_AUTH_TOKEN=your-generated-grpc-token
```

> **Note:** For Enterprise Edition, also set `TYK_AI_LICENSE=your-license-key`.

The full default configuration file contains extensive comments explaining every option. See the [Configuration Reference](#configuration-reference) below for key variables.

### Start AI Studio

```bash
sudo systemctl enable --now tyk-ai-studio
```

### Verify

```bash
sudo systemctl status tyk-ai-studio
curl -s http://localhost:8080/api/v1/health
```

View logs:

```bash
sudo journalctl -u tyk-ai-studio -f
```

---

## Part 2: Install Microgateway (Optional)

The Microgateway is the data plane component for hub-spoke deployments. It connects to AI Studio via gRPC to receive configuration and processes AI requests locally.

Skip this section if you're using AI Studio in standalone mode with its embedded gateway.

### Add Package Repository

**Debian / Ubuntu (DEB):**

```bash
# Community Edition
curl -s https://packagecloud.io/install/repositories/tyk/tyk-ai-microgateway/script.deb.sh | sudo bash

# Enterprise Edition (same repo as AI Studio EE — skip if already added above)
curl -s https://packagecloud.io/install/repositories/tyk/tyk-ee-unstable/script.deb.sh | sudo bash
```

**RHEL / CentOS / Amazon Linux (RPM):**

```bash
# Community Edition
curl -s https://packagecloud.io/install/repositories/tyk/tyk-ai-microgateway/script.rpm.sh | sudo bash

# Enterprise Edition (same repo as AI Studio EE — skip if already added above)
curl -s https://packagecloud.io/install/repositories/tyk/tyk-ee-unstable/script.rpm.sh | sudo bash
```

### Install the Package

**DEB:**

```bash
# Community Edition
sudo apt-get install tyk-microgateway

# Enterprise Edition
sudo apt-get install tyk-microgateway-ee
```

**RPM:**

```bash
# Community Edition
sudo yum install tyk-microgateway

# Enterprise Edition
sudo yum install tyk-microgateway-ee
```

The package installs:

| Path | Description |
|------|-------------|
| `/opt/tyk-microgateway/tyk-microgateway` | Server binary |
| `/opt/tyk-microgateway/mgw` | CLI tool |
| `/opt/tyk-microgateway/data/` | Data directory (SQLite database) |
| `/opt/tyk-microgateway/examples/analytics-pulse-config.yaml` | Analytics pulse example config |
| `/etc/default/tyk-microgateway` | Environment configuration (systemd) |
| `/lib/systemd/system/tyk-microgateway.service` | Systemd service unit |

### Configure Microgateway

Edit the environment configuration file:

```bash
sudo nano /etc/default/tyk-microgateway
```

At minimum, set these values:

```env
# Hub-Spoke: Edge Mode
GATEWAY_MODE=edge
CONTROL_ENDPOINT=your-studio-host:50051
EDGE_ID=edge-1
EDGE_NAMESPACE=default

# Security — MUST MATCH AI Studio values
EDGE_AUTH_TOKEN=must-match-studio-GRPC_AUTH_TOKEN
ENCRYPTION_KEY=must-match-studio-MICROGATEWAY_ENCRYPTION_KEY

# TLS — disable for initial setup, enable for production
EDGE_ALLOW_INSECURE=true
EDGE_TLS_ENABLED=false
```

> **Note:** For Enterprise Edition, also set `TYK_AI_LICENSE=your-license-key`.

### Configure Analytics Pulse

To send analytics data from the Microgateway back to the AI Studio control plane, configure the analytics pulse plugin.

Copy the example config:

```bash
sudo cp /opt/tyk-microgateway/examples/analytics-pulse-config.yaml /opt/tyk-microgateway/analytics-pulse-config.yaml
sudo chown tyk:tyk /opt/tyk-microgateway/analytics-pulse-config.yaml
```

The default configuration is:

```yaml
version: "1.0"

data_collection_plugins:
  - name: "analytics_pulse"
    enabled: true
    hook_types: ["analytics", "budget", "proxy_log"]
    replace_database: false
    priority: 100
    config:
      interval_seconds: 10
      max_batch_size: 1000
      max_buffer_size: 10000
      compression_enabled: true
      include_proxy_summaries: true
      include_request_response_data: true
      edge_retention_hours: 24
      excluded_vendors: ["mock", "test"]
      timeout_seconds: 30
      max_retries: 3
      retry_interval_secs: 5
```

Then enable it in `/etc/default/tyk-microgateway`:

```env
PLUGINS_CONFIG_PATH=/opt/tyk-microgateway/analytics-pulse-config.yaml
```

### Start Microgateway

```bash
sudo systemctl enable --now tyk-microgateway
```

### Verify

```bash
sudo systemctl status tyk-microgateway
curl -s http://localhost:8080/health
```

View logs:

```bash
sudo journalctl -u tyk-microgateway -f
```

Check the AI Studio logs for a successful edge gateway connection:

```bash
sudo journalctl -u tyk-ai-studio | grep -i "edge\|grpc"
```

---

## Database Setup

### PostgreSQL for AI Studio

AI Studio defaults to SQLite, which is fine for development. For production, use PostgreSQL:

```bash
# Install PostgreSQL (Ubuntu/Debian)
sudo apt-get install postgresql

# Create database and user
sudo -u postgres psql -c "CREATE USER tyk WITH PASSWORD 'your-db-password';"
sudo -u postgres psql -c "CREATE DATABASE tyk_ai_studio OWNER tyk;"
```

Then set in `/etc/default/tyk-ai-studio`:

```env
DATABASE_TYPE=postgres
DATABASE_URL=postgresql://tyk:your-db-password@localhost:5432/tyk_ai_studio?sslmode=require
```

### SQLite for Microgateway

The Microgateway uses SQLite by default, stored at `/opt/tyk-microgateway/data/microgateway.db`. No additional setup is required.

---

## Shared Secrets Reference

When running AI Studio with a Microgateway, these values **must match**:

| AI Studio Variable | Microgateway Variable | Purpose |
|---|---|---|
| `GRPC_AUTH_TOKEN` | `EDGE_AUTH_TOKEN` | Authenticates the gRPC connection |
| `MICROGATEWAY_ENCRYPTION_KEY` | `ENCRYPTION_KEY` | Encrypts synced configuration data |
| `TYK_AI_LICENSE` | `TYK_AI_LICENSE` | Enterprise license (if applicable) |

---

## Firewall Configuration

Open the following ports based on your deployment:

| Port | Component | Required |
|------|-----------|----------|
| 8080 | AI Studio (API + UI) | Always |
| 9090 | AI Studio (embedded gateway) | Standalone mode |
| 50051 | AI Studio (gRPC control server) | Hub-spoke mode |
| 8080 | Microgateway (proxy API) | Hub-spoke mode (on microgateway host) |

Example using `ufw`:

```bash
# AI Studio host
sudo ufw allow 8080/tcp
sudo ufw allow 9090/tcp
sudo ufw allow 50051/tcp   # Only if using hub-spoke mode

# Microgateway host (if separate machine)
sudo ufw allow 8080/tcp
```

Example using `firewalld`:

```bash
# AI Studio host
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --permanent --add-port=50051/tcp
sudo firewall-cmd --reload
```

---

## TLS Configuration (Production)

For production deployments, enable TLS on the gRPC connection between AI Studio and the Microgateway.

**AI Studio** (`/etc/default/tyk-ai-studio`):

```env
GRPC_TLS_INSECURE=false
GRPC_TLS_CERT_PATH=/etc/tyk-ai-studio/tls/server-cert.pem
GRPC_TLS_KEY_PATH=/etc/tyk-ai-studio/tls/server-key.pem
```

**Microgateway** (`/etc/default/tyk-microgateway`):

```env
EDGE_TLS_ENABLED=true
EDGE_ALLOW_INSECURE=false
# If using a private CA:
# EDGE_TLS_CA_PATH=/etc/tyk-microgateway/tls/ca-cert.pem
```

---

## Configuration Reference

### AI Studio Key Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SITE_URL` | `http://localhost:8080` | Public URL of the AI Studio UI |
| `DATABASE_TYPE` | `sqlite` | `sqlite` or `postgres` |
| `DATABASE_URL` | `midsommar.db` | Database connection string |
| `TYK_AI_SECRET_KEY` | — | Encryption key for secrets (required) |
| `MICROGATEWAY_ENCRYPTION_KEY` | — | Shared encryption key with microgateway |
| `GATEWAY_MODE` | `standalone` | `standalone` or `control` |
| `GRPC_PORT` | `50051` | gRPC server port (hub-spoke mode) |
| `GRPC_AUTH_TOKEN` | — | gRPC authentication token |
| `LOG_LEVEL` | `info` | `trace`, `debug`, `info`, `warn`, `error` |
| `ALLOW_REGISTRATIONS` | `true` | Allow new user sign-ups |
| `TYK_AI_LICENSE` | — | Enterprise license key |

### Microgateway Key Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `DATABASE_TYPE` | `sqlite` | `sqlite` or `postgres` |
| `GATEWAY_MODE` | `standalone` | `standalone` or `edge` |
| `CONTROL_ENDPOINT` | `localhost:50051` | AI Studio gRPC address |
| `EDGE_ID` | `edge-1` | Unique edge instance identifier |
| `EDGE_NAMESPACE` | `default` | Namespace for config partitioning |
| `EDGE_AUTH_TOKEN` | — | Must match AI Studio `GRPC_AUTH_TOKEN` |
| `ENCRYPTION_KEY` | — | Must match AI Studio `MICROGATEWAY_ENCRYPTION_KEY` |
| `PLUGINS_CONFIG_PATH` | — | Path to analytics pulse config YAML |
| `LOG_LEVEL` | `info` | `trace`, `debug`, `info`, `warn`, `error` |
| `TYK_AI_LICENSE` | — | Enterprise license key |

---

## Upgrading

**DEB:**

```bash
sudo apt-get update

# Community Edition
sudo apt-get upgrade tyk-ai-studio
sudo apt-get upgrade tyk-microgateway  # if installed

# Enterprise Edition
sudo apt-get upgrade tyk-ai-studio-ee
sudo apt-get upgrade tyk-microgateway-ee  # if installed
```

**RPM:**

```bash
# Community Edition
sudo yum update tyk-ai-studio
sudo yum update tyk-microgateway  # if installed

# Enterprise Edition
sudo yum update tyk-ai-studio-ee
sudo yum update tyk-microgateway-ee  # if installed
```

> **Note:** Package upgrades will **not** overwrite your configuration in `/etc/default/`. The services are automatically restarted after upgrade.

---

## Troubleshooting

### Service fails to start

```bash
sudo journalctl -u tyk-ai-studio --no-pager -n 50
```

Common causes:
- Missing or invalid `TYK_AI_SECRET_KEY`
- Database connection failure (check `DATABASE_URL`)
- Port already in use

### Permission errors

The services run as the `tyk` user. Ensure data directories are owned correctly:

```bash
sudo chown -R tyk:tyk /opt/tyk-ai-studio/
sudo chown -R tyk:tyk /opt/tyk-microgateway/
```

### SELinux issues (RHEL/CentOS)

If SELinux is enforcing and blocking the service:

```bash
sudo setsebool -P httpd_can_network_connect 1
# Or check audit log for specific denials:
sudo ausearch -m avc -ts recent
```

### Microgateway cannot connect to AI Studio

- Verify `CONTROL_ENDPOINT` points to the correct AI Studio host and gRPC port
- Verify `EDGE_AUTH_TOKEN` matches `GRPC_AUTH_TOKEN` exactly
- Verify `ENCRYPTION_KEY` matches `MICROGATEWAY_ENCRYPTION_KEY` exactly
- Check firewall rules allow traffic on the gRPC port (default 50051)
- Check AI Studio logs: `sudo journalctl -u tyk-ai-studio | grep grpc`

## Next Steps

Once deployed, proceed to the [Initial Configuration](./configuration.md) guide to set up your first LLM, users, and applications.
