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
    Replace `VARIABLE_NAME` with an environment variable name (e.g., `$ENV/MY_API_KEY`). This reads the value from AI Studio's environment at runtime, **but only for names the operator lists** in `AI_STUDIO_ENV_REF_ALLOWLIST`, a comma-separated list that is empty by default. AI Studio's environment also holds its own secrets, such as the encryption key and the database URL. An `$ENV/` reference to a name that isn't listed is never resolved: it stays as written, and nothing is sent in its place. List only the variables you inject for this purpose, for example Kubernetes secrets mounted as environment variables, or values provided by a vault agent or CI/CD pipeline. The list can't include itself.

    Store credentials in the secret store (`$SECRET/`) unless an external tool provides them through the environment.

*   **Who may write a reference:** Adding a `$SECRET/` or `$ENV/` reference to an LLM (API key, endpoint or metadata), a tool's auth key, a datasource's connection fields or an embedder needs read access to secrets (`secrets:read`). This also applies to changing one. The value is resolved and sent to the endpoint the writer configures. Editing an object and keeping its stored references needs nothing extra. Contributor submissions and plugins may not set references at all: they are refused with `400` and `InvalidArgument`.

*   **Edge gateways** never resolve references themselves. AI Studio resolves what the policy allows into the configuration it pushes. If AI Studio refuses a reference in a key or token, edges receive the placeholder `refused-credential-reference` instead. The provider rejects it, and the edge never substitutes a key from its own environment. A refused endpoint or connection string reaches edges as a bare `$ENV/`, which resolves to nothing.

*   **Runtime Resolution:** When a configuration uses a field containing a secret reference (e.g., `$SECRET/MY_KEY`):
    1.  The configuration itself stores the `$SECRET/MY_KEY` string, *not* the actual secret value.
    2.  Only when the system needs the actual value at runtime (e.g., the [Proxy](./proxy.md) preparing a request for an LLM, or a [Tool](./tools.md) calling its external API), Tyk AI Studio retrieves the encrypted secret value, decrypts it using the `TYK_AI_SECRET_KEY`, and injects the plain text value into the operation.
    3.  The decrypted value is typically used immediately and not persisted further.

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
