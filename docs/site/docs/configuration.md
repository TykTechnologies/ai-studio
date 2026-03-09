---
title: "Initial Configuration"
weight: 2
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# Initial Configuration

This guide covers the essential first steps to take within the Tyk AI Studio UI after successfully deploying the platform using one of the installation methods:

- [Docker Compose](./deployment-docker.md)
- [Kubernetes / Helm](./deployment-helm-k8s.md)
- [Bare Metal / VM (DEB/RPM Packages)](./deployment-packages.md)

## 1. First Login

1.  **Access the UI:** Open your web browser and navigate to the `SITE_URL` specified during deployment (e.g., `http://localhost:8080`).
2.  **Register an account:** If `ALLOW_REGISTRATIONS=true` (the default for new deployments), click **Register** to create your admin account. The first registered user automatically becomes the administrator.

## 2. Add Your LLM API Keys

Tyk AI Studio pre-populates OpenAI and Anthropic LLM configurations on first startup. These are already wired to placeholder secrets (`OPENAI_KEY` and `ANTHROPIC_KEY`) — you just need to fill in your actual API keys.

1.  **Navigate to Secrets:** In the admin UI sidebar, go to **Governance → Secrets**.
2.  **Edit `OPENAI_KEY`:** Click on the `OPENAI_KEY` secret, then click **Edit** and paste your OpenAI API key.
3.  **Edit `ANTHROPIC_KEY`:** Do the same for `ANTHROPIC_KEY` with your Anthropic API key.

The pre-created LLM configs reference these secrets using the format `$SECRET/OPENAI_KEY`. For more on secrets management, see the [Secrets](./secrets.md) documentation.

### Adding More LLM Providers

To connect additional providers (Google Vertex AI, Ollama, Azure, etc.):

1.  Navigate to **LLM Management** in the sidebar and click **Add LLM Configuration**.
2.  Select the provider and enter the configuration details (name, model identifiers, base URL if applicable).
3.  For the API key, create a new secret in **Governance → Secrets** and reference it as `$SECRET/YOUR_SECRET_NAME` in the API Key field.

For more details, see the [LLM Management](./llm-management.md) documentation.

## 3. Push Configuration to Edge Gateways

If you are running a Microgateway (hub-spoke deployment), you need to push the updated configuration after adding your API keys:

1.  Navigate to **AI Portal → Edge Gateways** in the sidebar.
2.  Verify your edge gateway shows as **Connected**.
3.  Click **Push Configuration** to sync the latest settings.

Once the sync status shows **Synced**, the Microgateway is ready to proxy LLM requests. This LLM is now available for use within Tyk AI Studio, subject to [User/Group permissions](./user-management.md).

## 4. Verify Core System Settings

While most core settings are configured during deployment, you can usually review them within the administration UI:

*   **Site URL:** Check that the base URL for accessing the portal is correct.
*   **Email Configuration:** If using features like user invites or notifications, ensure SMTP settings are correctly configured and test email delivery if possible ([Notifications](./notifications.md)).

## 5. Configuration Reference (Deployment)

Remember that fundamental system parameters are typically set via environment variables or Helm values *during deployment*. This includes:

### Core System Settings
*   Database Connection (`DATABASE_TYPE`, `DATABASE_URL`)
*   License Key (`TYK_AI_LICENSE`)
*   Secrets Encryption Key (`TYK_AI_SECRET_KEY`)
*   Base URL (`SITE_URL`)
*   Email Server Settings (`SMTP_*`, `FROM_EMAIL`)
*   Registration Settings (`ALLOW_REGISTRATIONS`, `FILTER_SIGNUP_DOMAINS`)

### Message Queue Configuration
*   Queue Type (`QUEUE_TYPE`): `inmemory` (default), `nats`, or `postgres`
*   Buffer Size (`QUEUE_BUFFER_SIZE`): Default 100

### NATS Configuration (when QUEUE_TYPE=nats)
*   **Connection**: `NATS_URL`, `NATS_STORAGE_TYPE`, `NATS_RETENTION_POLICY`
*   **Authentication**: `NATS_USERNAME`/`NATS_PASSWORD`, `NATS_TOKEN`, `NATS_CREDENTIALS_FILE`
*   **Security**: `NATS_TLS_ENABLED`, `NATS_TLS_CERT_FILE`, `NATS_TLS_KEY_FILE`
*   **Performance**: `NATS_MAX_AGE`, `NATS_MAX_BYTES`, `NATS_ACK_WAIT`

For detailed NATS configuration options, see the [NATS Configuration Guide](./nats-configuration.md).

Refer to the **Configuration Options** detailed within the [Installation Guide](./deployment-helm-k8s.md) for specifics on setting these values during the deployment process.

## 6. Namespace Support (Enterprise)

> **Note:** Namespace support is an **Enterprise Edition** feature for hub-and-spoke deployments.

Namespaces allow you to partition resources across distributed deployments, enabling multi-tenant or geographically distributed architectures.

### What Are Namespaces?

> **Edition Note:** Community Edition supports a **single namespace** only. Enterprise Edition supports multiple namespaces for multi-tenant and geographically distributed deployments.

A namespace is a logical grouping that isolates resources within a Tyk AI Studio deployment. Resources that support namespaces include:

*   **LLM Configurations** - Partition LLM access by region or tenant
*   **Apps** - Scope applications to specific namespaces
*   **Filters** - Apply different filter policies per namespace
*   **Plugins** - Deploy plugins to specific edge instances
*   **Model Routers** - Configure routing rules per namespace
*   **Agent Configs** - Scope AI agents to namespaces

### Hub-and-Spoke Architecture

In enterprise deployments, Tyk AI Studio supports a hub-and-spoke model:

*   **Hub (Control Plane):** Central Tyk AI Studio instance managing configuration, policies, and analytics
*   **Spoke (Edge Instances):** Distributed Microgateway instances processing requests locally

Namespaces enable the hub to push configuration to specific edge instances, allowing:

*   **Regional Compliance:** Keep data processing within specific geographic regions
*   **Multi-Tenancy:** Isolate resources between different teams or customers
*   **Distributed Processing:** Route requests to the nearest edge instance

### Configuration

When creating resources (LLMs, Apps, Filters, etc.), you can specify a `namespace` field to associate them with a specific edge instance or group of instances.

```json
{
  "name": "OpenAI Config - EU",
  "vendor": "openai",
  "namespace": "eu-west-1",
  ...
}
```

Edge instances register with the hub using their namespace identifier, and only receive configuration relevant to their namespace.

## 7. Enable the Plugin Marketplace

The Plugin Marketplace lets you browse and install community plugins directly from the admin UI. It is enabled by default (`MARKETPLACE_ENABLED=true`), but requires the `AI_STUDIO_OCI_CACHE_DIR` environment variable to be set. **Without it, the Marketplace page will appear empty.**

Set this variable in your deployment configuration:

| Deployment Method | Where to Set |
|---|---|
| Docker Compose | Add `AI_STUDIO_OCI_CACHE_DIR=./data/cache/plugins` to your `studio.env` |
| Helm / Kubernetes | Set `config.ociCacheDir: "./cache/plugins"` in your values file |
| Bare Metal / VM | Add `AI_STUDIO_OCI_CACHE_DIR=/opt/tyk-ai-studio/cache/plugins` to `/etc/default/tyk-ai-studio` |

Restart AI Studio after making this change. On startup, the marketplace service will automatically sync the default plugin index.

## Next Steps

With the initial configuration complete, you can now:

*   Explore [User Management](./user-management.md) to create users and groups.
*   Set up [Tools](./tools.md) for external API integration.
*   Configure [Data Sources](./datasources-rag.md) for RAG.
*   Define [Filters](./filters.md) for custom request/response logic.
*   Try out the [Chat Interface](./chat-interface.md).
