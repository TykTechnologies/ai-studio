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

This guide covers the essential first steps to take within the Tyk AI Studio UI after successfully deploying the platform using the [Installation Guide](./deployment-helm-k8s.md).

## 1. First Login

1.  **Access the UI:** Open your web browser and navigate to the `SITE_URL` (or the Ingress host configured for `app.yourdomain.com`) specified during deployment.
2.  **Admin Credentials:** Log in using the initial administrator credentials. This is typically:
    *   **Email:** The `ADMIN_EMAIL` configured during deployment (e.g., `admin@yourdomain.com`).
    *   **Password:** Check the deployment logs or default configuration for the initial admin password setup. You may be required to set a new password upon first login.

## 2. Configure Your First LLM

One of the most common initial steps is connecting Tyk AI Studio to an LLM provider.

1.  **Navigate to LLM Management:** In the admin UI sidebar, find the section for LLM Management (or similar) and select it.
2.  **Add LLM Configuration:** Click the button to add a new LLM Configuration.
3.  **Select Provider:** Choose the LLM provider you want to connect (e.g., OpenAI, Anthropic, Google Vertex AI, Ollama).
4.  **Enter Details:**
    *   **Configuration Name:** Give it a recognizable name (e.g., `OpenAI-GPT-4o`).
    *   **Model Name(s):** Specify the exact model identifier(s) provided by the vendor (e.g., `gpt-4o`, `gpt-4-turbo`).
    *   **API Key (Using Secrets):**
        *   **IMPORTANT:** Do *not* paste your API key directly here. Instead, use [Secrets Management](./secrets.md).
        *   If you haven't already, go to the **Secrets** section in the admin UI and create a new secret:
            *   **Variable Name:** `OPENAI_API_KEY` (or similar)
            *   **Secret Value:** Paste your actual OpenAI API key here.
            *   Save the secret.
        *   Return to the LLM Configuration screen.
        *   In the API Key field, enter the secret reference: `$SECRET/OPENAI_API_KEY` (using the exact Variable Name you created).
    *   **Other Parameters:** Configure any other provider-specific settings (e.g., Base URL for Azure/custom endpoints, default temperature, etc.).
5.  **Save:** Save the LLM configuration.

This LLM is now available for use within Tyk AI Studio, subject to [User/Group permissions](./user-management.md).

For more details, see the [LLM Management](./llm-management.md) documentation.

## 3. Verify Core System Settings

While most core settings are configured during deployment, you can usually review them within the administration UI:

*   **Site URL:** Check that the base URL for accessing the portal is correct.
*   **Email Configuration:** If using features like user invites or notifications, ensure SMTP settings are correctly configured and test email delivery if possible ([Notifications](./notifications.md)).

## 4. Configuration Reference (Deployment)

Remember that fundamental system parameters are typically set via environment variables or Helm values *during deployment*. This includes:

### Core System Settings
*   Database Connection (`DATABASE_TYPE`, `DATABASE_URL`)
*   License Key (`TYK_AI_LICENSE`)
*   Secrets Encryption Key (`TYK_AI_SECRET_KEY`)
*   Base URL (`SITE_URL`)
*   Email Server Settings (`SMTP_*`, `FROM_EMAIL`, `ADMIN_EMAIL`)
*   Registration Settings (`ALLOW_REGISTRATIONS`, `FILTER_SIGNUP_DOMAINS`)

### Message Queue Configuration
*   Queue Type (`QUEUE_TYPE`): `inmemory` (default) or `nats`
*   Buffer Size (`QUEUE_BUFFER_SIZE`): Default 100

### NATS Configuration (when QUEUE_TYPE=nats)
*   **Connection**: `NATS_URL`, `NATS_STORAGE_TYPE`, `NATS_RETENTION_POLICY`
*   **Authentication**: `NATS_USERNAME`/`NATS_PASSWORD`, `NATS_TOKEN`, `NATS_CREDENTIALS_FILE`
*   **Security**: `NATS_TLS_ENABLED`, `NATS_TLS_CERT_FILE`, `NATS_TLS_KEY_FILE`
*   **Performance**: `NATS_MAX_AGE`, `NATS_MAX_BYTES`, `NATS_ACK_WAIT`

For detailed NATS configuration options, see the [NATS Configuration Guide](./nats-configuration.md).

Refer to the **Configuration Options** detailed within the [Installation Guide](./deployment-helm-k8s.md) for specifics on setting these values during the deployment process.

## 5. Namespace Support (Enterprise)

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

## Next Steps

With the initial configuration complete, you can now:

*   Explore [User Management](./user-management.md) to create users and groups.
*   Set up [Tools](./tools.md) for external API integration.
*   Configure [Data Sources](./datasources-rag.md) for RAG.
*   Define [Filters](./filters.md) for custom request/response logic.
*   Try out the [Chat Interface](./chat-interface.md).
