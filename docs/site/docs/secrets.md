---
title: "Secrets Management"
weight: 50
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# Secrets Management

Tyk AI Studio provides a secure mechanism for storing and managing sensitive information, such as API keys, passwords, authentication tokens, or other credentials required by various platform configurations.

## Purpose

The Secrets Management system aims to:

*   **Prevent Exposure:** Avoid hardcoding sensitive values directly in configuration files or UI fields.
*   **Centralize Management:** Provide a single place to manage and update credentials.
*   **Enhance Security:** Store sensitive data encrypted at rest.

## Core Concepts

*   **Secret:** A named key-value pair where the 'key' is the **Variable Name** used for reference, and the 'value' is the actual sensitive data (e.g., an API key string).

*   **Encryption:**
    *   Secret values are **encrypted at rest** within Tyk AI Studio's storage.
    *   The encryption and decryption process relies on a key derived from the **`TYK_AI_SECRET_KEY` environment variable** set when running the Tyk AI Studio instance.
    *   **CRITICAL:** The `TYK_AI_SECRET_KEY` must be kept confidential and managed securely. Loss of this key will render existing secrets unusable. Changing the key will require re-entering all secrets.

*   **Reference Syntax:** Sensitive values can be referenced within configuration fields using two syntaxes:

    **Database-stored secrets:**
    ```
    $SECRET/VariableName
    ```
    Replace `VariableName` with the exact name given to the secret when it was created (e.g., `$SECRET/OPENAI_API_KEY`, `$SECRET/JIRA_AUTH_TOKEN`). These values are stored encrypted in the database.

    **Environment variables:**
    ```
    $ENV/VARIABLE_NAME
    ```
    Replace `VARIABLE_NAME` with an environment variable name (e.g., `$ENV/MY_API_KEY`). This reads the value from AI Studio's environment at runtime, **but only for names the operator lists** in `AI_STUDIO_ENV_REF_ALLOWLIST`, a comma-separated list that is empty by default. AI Studio's environment also holds its own secrets, such as the encryption key and the database URL. An `$ENV/` reference to a name that isn't listed is never resolved: it stays as written. The variable's value is never read. An LLM key, embedder key or REST tool auth key that is still a reference is never sent: the gateway answers `502` (`LLM credential not usable` on the REST and streaming routes), and Studio Chat, embedders and tool calls fail with `credential is an unresolved reference`. Other credentials (such as a datasource connection string) are used as written, so the provider rejects the call (usually with `401`). See [Upgrading from 2.2](#upgrading-from-2-2) for how this shows up. List only the variables you inject for this purpose, for example Kubernetes secrets mounted as environment variables, or values provided by a vault agent or CI/CD pipeline. The list can't include itself.

    Store credentials in the secret store (`$SECRET/`) unless an external tool provides them through the environment.

*   **Who may write a reference:** Adding a `$SECRET/` or `$ENV/` reference to an LLM (API key, endpoint or metadata), a tool's auth key, a datasource's connection fields or an embedder needs read access to secrets (`secrets:read`). This also applies to changing one. The value is resolved and sent to the endpoint the writer configures. Editing an object and keeping its stored references needs nothing extra. Contributor submissions and plugins may not set references at all: they are refused with `400` and `InvalidArgument`.

*   **Edge gateways** never resolve references themselves. AI Studio resolves what the policy allows into the configuration it pushes. If AI Studio refuses a reference in a key or token, cannot resolve it, or it names a secret stored with an empty value, edges receive the placeholder `refused-credential-reference` instead. A 2.3 edge refuses an LLM, embedder or REST tool call that would send the placeholder, as AI Studio does, rather than passing it to the vendor. This applies to LLM API keys and metadata credentials, tool auth keys, datasource and embedder keys. A key field left empty (an Ollama LLM without a key) is still sent empty. The provider rejects it, and the edge never substitutes a key from its own environment. A refused endpoint or connection string reaches edges as a bare `$ENV/`, which resolves to nothing. Plugins that run on edges get their top-level `$SECRET/` config values the same way, encrypted (see [Plugin Security](./plugins-security.md#plugin-configuration-and-secrets)). Creating, changing or deleting a secret marks edges pending when their configuration uses it; the new value reaches them on the next push.

*   **LLM credentials typed in directly** (from 2.3.0): an LLM's API key, and the values of its metadata fields whose name contains `key`, `secret` or `token` (a Bedrock LLM's `aws_access_key_id`, `aws_secret_access_key` and `aws_session_token`), are stored encrypted with `TYK_AI_SECRET_KEY` when you enter the value itself rather than a `$SECRET/` or `$ENV/` reference. LLMs saved before 2.3.0 are encrypted when AI Studio starts.
    *   **Without `TYK_AI_SECRET_KEY`** these values are stored as written, and AI Studio logs a security warning at startup. Set the key and restart to encrypt them.
    *   **If the key changes, or goes missing,** stored LLM credentials can no longer be read. They are refused: the call fails before it reaches the provider (`LLM credential not usable` on the gateway, `credential is an unresolved reference` in Studio Chat), and the error is logged. Re-enter the credentials, or set the key back.
    *   **Edges** receive these values encrypted with `MICROGATEWAY_ENCRYPTION_KEY`, as they receive API keys, and store them encrypted. An edge older than 2.3.0 does not read them: it gets `refused-credential-reference` for a Bedrock LLM's metadata credentials and its Bedrock calls fail. Upgrade edges with AI Studio.
    *   **They do not leave AI Studio in clear.** The admin API, plugin events (`system.llm.*`), object hooks and the cluster event log carry `[redacted]` in their place (references are shown as written), and SQL logs never include values unless `DB_LOG_SQL_PARAMS=true` (see [Configuration](./configuration.md)).

*   **Runtime Resolution:** When a configuration uses a field containing a secret reference (e.g., `$SECRET/MY_KEY`):
    1.  The configuration itself stores the `$SECRET/MY_KEY` string, *not* the actual secret value.
    2.  Only when the system needs the actual value at runtime (e.g., the [Proxy](./proxy.md) preparing a request for an LLM, or a [Tool](./tools.md) calling its external API), Tyk AI Studio retrieves the encrypted secret value, decrypts it using the `TYK_AI_SECRET_KEY`, and injects the plain text value into the operation.
    3.  The decrypted value is typically used immediately and not persisted further.

## Upgrading from 2.2

Before 2.3.0, every `$ENV/` reference resolved against AI Studio's environment. From 2.3.0, only names in `AI_STUDIO_ENV_REF_ALLOWLIST` do, and the list is empty by default. **Before you upgrade**, list the names your configuration uses in `AI_STUDIO_ENV_REF_ALLOWLIST`, or move those credentials to the secret store as `$SECRET/` references.

References are stored as written, so a database dump shows every name in use:

```bash
# PostgreSQL
pg_dump --data-only "$DATABASE_URL" | grep -oE '\$ENV/[A-Za-z_][A-Za-z0-9_]*' | sort -u
# SQLite
sqlite3 midsommar.db .dump | grep -oE '\$ENV/[A-Za-z_][A-Za-z0-9_]*' | sort -u
```

The output can include `$ENV/` references in plugin configuration, which the plugin resolves against its own environment (see [Plugin Security](./plugins-security.md)), not this list.

If you upgrade without doing this, an LLM, tool, datasource, embedder or guardrail connection whose credential is an unlisted `$ENV/` reference behaves as follows:

- **The console reports the LLM's credential as unresolved** and says the environment variable "is not set on the server", even when it is set. The variable is set; its name is not on the list.
- **Calls fail.** An LLM key, embedder key or REST tool auth key that is still a reference is never sent: the gateway (AI Studio's or an edge's) answers `502` (`LLM credential not usable`), and Studio Chat, embedders and tool calls fail with `credential is an unresolved reference`. Other credentials, such as a datasource connection string, are sent as written and the provider answers `401`. A guardrail applies its fail mode.
- **Nothing is logged.** Neither startup nor the failing requests mention the allowlist, and saving a new unlisted reference succeeds without a warning.

To fix it, add the names to `AI_STUDIO_ENV_REF_ALLOWLIST` and restart AI Studio, or change the credentials to `$SECRET/` references.

## Creating & Managing Secrets (Admin)

Administrators manage secrets via the Tyk AI Studio UI or API:

1.  Navigate to the Secrets management section.
2.  Create a new secret by providing:
    *   **Variable Name:** A unique identifier (letters, numbers, underscores) used in the `$SECRET/VariableName` reference.
    *   **Secret Value:** The actual sensitive string (e.g., `sk-abc123xyz...`).
3.  Save the secret. It is immediately encrypted and stored.

Secrets can be updated or deleted as needed. Updating a secret value will automatically apply the new value wherever the `$SECRET/VariableName` reference is used, without needing to modify the configurations themselves.

## Usage Examples

Secrets are commonly used in:

*   **[LLM Configurations](./llm-management.md):** Storing API keys for providers like OpenAI, Anthropic, Google Vertex AI, etc.
    *   *Example:* In the API Key field for an OpenAI configuration: `$SECRET/OPENAI_API_KEY`
*   **[Tool Configurations](./tools.md):** Storing API keys, authentication tokens (Bearer, Basic Auth), or other credentials needed to interact with the external API the tool represents.
    *   *Example:* In a field for a custom header for a JIRA tool: `Authorization: Basic $SECRET/JIRA_BASIC_AUTH_TOKEN`
*   **[Data Source Configurations](./datasources-rag.md):** Storing API keys or connection credentials for vector databases (e.g., Pinecone, Milvus) or embedding service providers.
    *   *Example:* In the API Key field for a Pinecone vector store: `$SECRET/PINECONE_API_KEY`

## Security Considerations

*   **Protect `TYK_AI_SECRET_KEY`:** This is the master key for secrets. Treat it with the same level of security as database passwords or root credentials. Use environment variable management best practices.
*   **Principle of Least Privilege:** Grant administrative access (which includes secrets management) only to trusted users.
*   **Regular Rotation:** Consider policies for regularly rotating sensitive credentials by updating the Secret Value in Tyk AI Studio.
