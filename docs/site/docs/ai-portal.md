---
title: "AI Portal"
weight: 80
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# AI Portal

The Tyk AI Studio's AI Portal provides a user-friendly web interface where end users can interact with configured AI capabilities. It serves as the primary access point for users to engage with Chat Experiences, view documentation, and manage their account settings.

## Purpose

The main goals of the AI Portal are:

*   **Unified User Experience:** Offer a cohesive interface for accessing all AI capabilities configured within Tyk AI Studio.
*   **Self-Service Access:** Enable users to independently access and utilize AI features without administrator intervention.
*   **Contextual Documentation:** Provide integrated documentation and guidance for available AI services.
*   **Account Management:** Allow users to manage their own profile settings and view usage information.
*   **Secure Access Control:** Enforce permissions based on user groups and organizational policies.

## Key Features

*   **Chat Interface:** Access to all [Chat Experiences](./chat-interface.md) the user has permission to use, with a clean, intuitive UI for conversational interactions.
*   **Overview:** The developer's landing page: every app they own with its status (Active, Awaiting approval, Disabled), spend against its monthly budget, when it last reached the gateway and how many requests it made in the last 30 days, followed by what they can build with (counts per asset type and the newest assets).
*   **Browse:** One searchable catalog of every LLM provider, data source, tool and plugin resource the user's teams can use, newest first, with filters for type, vendor or store, privacy level, catalog and community submissions. Every asset has a detail page (models and allow list, privacy level, endpoint base URL, the catalogs it comes through, the user's apps that already use it); catalogs are a filter, not a level of navigation.
*   **Documentation Hub:** Integrated documentation for available AI services, tools, and data sources.
*   **User Profile Management:** Self-service capabilities for updating profile information and preferences.
*   **History & Favorites:** Access to past chat sessions and ability to bookmark favorite conversations.
*   **Responsive Design:** Optimized for both desktop and mobile devices for consistent access across platforms.
*   **Customizable Themes:** Support for light/dark mode and potentially organization-specific branding.
*   **Notifications:** System alerts and updates relevant to the user's AI interactions.

## Using the AI Portal

Users access the AI Portal through a web browser at the configured URL for their Tyk AI Studio installation.

1.  **Authentication:** Users log in using their credentials (username/password, SSO, or other configured authentication methods).
2.  **Overview:** Upon login, developers land on the Overview: their apps (status, budget, last access) and the assets they can build with, with a search box that opens Browse.
3.  **Browse:** Users search and filter one catalog of everything their teams can use, open an asset's detail page, and start an app from it ("Build app", or "Get access" for a data source). The old per-catalog pages redirect into Browse filtered by that catalog.
4.  **Apps:** Users create apps, wait for an administrator to approve the credential, and read the endpoint documentation on the app page.
5.  **Chat Selection:** Users can select from available Chat Experiences to start or continue conversations.
6.  **Documentation Access:** Users can browse integrated documentation to learn about available capabilities.
7.  **Profile Management:** Users can update their profile settings, preferences, and view usage statistics.

## Configuration (Admin)

Administrators configure the AI Portal through the Tyk AI Studio admin interface:

*   **Portal Branding:** Customize logos, colors, and themes to match organizational branding.
*   **Available Features:** Enable or disable specific portal features (chat, documentation, etc.).
*   **Authentication Methods:** Configure login options (local accounts, SSO integration, etc.).
*   **Default Settings:** Set system-wide defaults for user experiences.
*   **Access Control:** Manage which user groups can access the portal and specific features within it.
*   **Custom Content:** Add organization-specific documentation, welcome messages, or announcements.

## API Access

While the AI Portal primarily provides a web-based user interface, it is built on top of the same APIs that power the rest of Tyk AI Studio. Developers can access these APIs directly for custom integrations:

*   **Authentication API:** `/api/v1/auth/...` endpoints for managing user sessions.
*   **Chat API:** `/api/v1/chat/...` endpoints for programmatic access to chat functionality.
*   **User Profile API:** `/api/v1/users/...` endpoints for managing user information.
*   **Datasource API:** `/datasource/{dsSlug}` endpoints for querying configured data sources, generating embeddings, and metadata filtering.

### Datasource API

The Datasource API provides direct access to configured vector stores for semantic search, vector-based search, metadata filtering, and embedding generation.

#### Text Search

Perform semantic search using a natural language query. The query text is automatically converted to an embedding vector.

*   **Endpoint:** `POST /datasource/{dsSlug}`
*   **Authentication:** Bearer token required
*   **Request Format:**
    ```json
    {
      "query": "your search query here",
      "n": 5  // optional, number of results to return (default: 3)
    }
    ```
*   **Response Format:**
    ```json
    {
      "documents": [
        {
          "PageContent": "text content of the document chunk",
          "Metadata": {
            "source": "filename.pdf",
            "page": 42
          },
          "Score": 0.92
        }
      ]
    }
    ```

#### Vector Search

Perform similarity search using a pre-computed embedding vector. Useful when you have already generated embeddings or want to use a custom embedding strategy.

*   **Endpoint:** `POST /datasource/{dsSlug}/vector`
*   **Authentication:** Bearer token required
*   **Request Format:**
    ```json
    {
      "embedding": [0.1, 0.2, 0.3, ...],
      "n": 10,                    // optional, max results (default: 10)
      "similarity_threshold": 0.7  // optional, minimum score filter (default: 0.0)
    }
    ```
*   **Response Format:**
    ```json
    {
      "documents": [
        {
          "PageContent": "text content of the document chunk",
          "Metadata": {
            "source": "filename.pdf",
            "page": 42
          },
          "Score": 0.92
        }
      ]
    }
    ```

#### Metadata Query

Query documents using metadata filters only (no vector similarity search). Supports pagination.

*   **Endpoint:** `POST /datasource/{dsSlug}/metadata`
*   **Authentication:** Bearer token required
*   **Request Format:**
    ```json
    {
      "filter": {
        "source": "filename.pdf",
        "category": "technical"
      },
      "filter_mode": "AND",  // optional, "AND" or "OR" (default: "AND")
      "limit": 10,           // optional, max results (default: 10, max: 100)
      "offset": 0            // optional, pagination offset (default: 0)
    }
    ```
*   **Response Format:**
    ```json
    {
      "documents": [
        {
          "PageContent": "text content of the document chunk",
          "Metadata": {
            "source": "filename.pdf",
            "category": "technical"
          },
          "Score": 0.0
        }
      ],
      "total_count": 42
    }
    ```

#### Generate Embeddings

Generate embedding vectors for text chunks without storing them. The datasource must have an embedder configured.

*   **Endpoint:** `POST /datasource/{dsSlug}/embeddings`
*   **Authentication:** Bearer token required
*   **Request Format:**
    ```json
    {
      "texts": ["first text chunk", "second text chunk"]  // max 100 items
    }
    ```
*   **Response Format:**
    ```json
    {
      "vectors": [
        [0.1, 0.2, 0.3, ...],
        [0.4, 0.5, 0.6, ...]
      ]
    }
    ```

**Important Note:** Datasource endpoints do not accept a trailing slash. Use `/datasource/{dsSlug}` not `/datasource/{dsSlug}/`.

This API-first approach ensures that all functionality available through the AI Portal can also be accessed programmatically for custom applications or integrations.

The AI Portal serves as the primary touchpoint for end users interacting with AI capabilities managed by Tyk AI Studio, providing a secure, intuitive, and feature-rich experience.
