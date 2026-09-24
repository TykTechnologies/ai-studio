# Embedders

An **Embedder** is a saved embedding configuration: which client to call, where, with which credentials and which model. [Data Sources](./datasources-rag.md) embed their documents and queries with one, [Semantic Routers](./semantic-router.md) embed their example prompts and requests with one, and any number of them can share the same embedder.

Most people never need to open the Embedders page: when you configure a data source, pick an existing embedder or create one inline. The page is there to review, fix and clean up embedders.

## Linked or standalone

| | Linked to an LLM | Standalone |
|---|---|---|
| Connection | Uses the LLM provider's vendor, endpoint and API key | Its own **API compatibility**, endpoint and API key |
| Privacy score | The LLM provider's | Its own |
| Good for | Reusing a provider you already configured (for example OpenAI) | A dedicated embedding service, such as a local model behind an OpenAI-compatible API |

**API compatibility** is the client AI Studio uses, not who serves the model. A Hugging Face model served through an OpenAI-compatible endpoint uses `openai`. The choices are the vendors whose drivers support embeddings: OpenAI, Ollama, Google AI, Vertex and Hugging Face. For Vertex, the endpoint is `project:location`.

API keys can be [Secrets](./secrets.md) references (`$SECRET/NAME`), as on LLM providers.

## Rules that protect your data

- **The model is locked while data sources use it.** A data source's stored vectors came from its embedder's model; embedding new queries with another model would silently return poor matches. To change model, create a new embedder, point the data source at it and re-process its embeddings. You can always rotate the endpoint or key.
- **Privacy.** An embedder sees the text it embeds, so its privacy score must be at least the data source's.
- **Deleting.** An embedder in use cannot be deleted; the error lists the data sources and semantic routers that use it. An LLM provider that embedders are linked to cannot be deleted either.

## API

`/api/v1/embedders` supports list, create, get, update (`PATCH`), delete and `GET /embedders/{id}/dependents`. `GET /api/v1/embedders/vendors` lists the API compatibilities. Keys are never returned; send `"[redacted]"` on update to keep the stored key.

The data source API is unchanged: `embed_vendor`, `embed_url`, `embed_api_key` and `embed_model` still work, and responses add `embedder_id`. Data source writes may send `embedder_id` instead of the `embed_*` fields. When the `embed_*` fields are used, AI Studio links the data source to an embedder with exactly that configuration, creating one if needed. It never edits an embedder that other data sources share.

## Permissions

The `embedders` permission (LLM management group) controls this page and API. Creating an embedder from the data source form also needs `embedders:write`.

## Upgrading

On first start after upgrading, AI Studio moves each data source's embedding settings onto embedders. Data sources with identical settings share one embedder, which takes the highest privacy score among them. Semantic Routers that named an embedding LLM and model are linked to an embedder that uses that LLM with that model. Nothing needs to be re-indexed.

The Semantic Router API still accepts `settings.embedding` as `{llm_id, model}` (saved as the matching linked embedder) and still returns that shape, alongside `embedder_id` and `embedder_name`.

Edges receive a router's embedder inside its configuration. Edges older than this release understand embedders that use an LLM provider, but not standalone ones; upgrade edges before giving a router a standalone embedder.
