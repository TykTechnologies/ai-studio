# Embedders

An **Embedder** is a reusable embedding configuration: the client (API compatibility), endpoint, credentials and model that turn text into vectors. Datasources embed with one (and, from the Semantic Router switch-over, so do Semantic Routers). Before Embedders, each datasource carried its own `embed_vendor/url/api_key/model` fields and Semantic Routers named an LLM plus a model; the list of vendors that can embed existed three times and had drifted.

Embedders are managed in the admin UI and API only. They are not exposed in the AI Portal, and neither the plugin SDK nor the proto changed.

## Model

`models.Embedder` (`models/embedder.go`):

| Field | Notes |
|---|---|
| `name` | Unique. Hard delete, so a name can be reused after its embedder is deleted. |
| `llm_id` | Set = **linked**: vendor, endpoint, key and privacy score come from that LLM, live. |
| `vendor` | Standalone only. The **API compatibility** (the client used), not necessarily who serves the model: an OpenAI-compatible endpoint in front of a Hugging Face model is `openai`. |
| `endpoint` | Standalone only. Base URL; for Vertex, `project:location`. |
| `api_key` | Standalone only. Plain value or `$SECRET/` / `$ENV/` reference, as on LLMs. Indexed in `secret_references` (object type `embedder`). |
| `model` | Always required (column `model`, Go field `ModelName`). |
| `privacy_score` | Standalone only; a linked embedder uses its LLM's. |

`Datasource.EmbedderID` references it. The old `embed_*` columns stay in the table, cleared, until a later release drops them; the Go fields are gone, so no code can read them.

**Which vendors can embed** has one source: the vendor drivers' `ProvidesEmbedder()`. `switches.EmbeddingVendors()` derives the list (OpenAI, Ollama, Google AI, Vertex, Hugging Face today; not Anthropic, Bedrock). `GET /vendors/embedders` and `GET /embedders/vendors` serve it; `switches.AVAILABLE_EMBEDDERS` is gone, which also adds Hugging Face to the datasource vendor list.

## Runtime resolution

Runtime code never reads embedder fields directly. It takes a resolved `models.EmbedderSpec` (vendor, endpoint, key, model, privacy score): `Embedder.Spec(resolveSecrets)` or `models.GetEmbedderResolved(db, id)`. A linked embedder needs its LLM preloaded; a linked embedder whose LLM is gone does not resolve (`ErrEmbedderLLMMissing`).

- Every datasource loader preloads `Embedder.LLM` (`models/datasource.go`, chat default datasource, portal catalogue).
- `data_session.getEmbedder` builds the client from the datasource's embedder and errors clearly when there is none.
- `switches.GetEmbedder(spec)` and the vendor interface `GetEmbedder(*EmbedderSpec)` take the spec. The Vertex embedder reads `project:location` from the endpoint (it used to read the vector store's connection string, so it only worked with a Vertex store).
- Secrets resolve when the spec is taken, so the Studio gateway's `/datasource/*` endpoints now reach the vendor with a `$SECRET/` key's value (they used to pass the reference string).

## Legacy datasource fields (facade)

The datasource REST API, gRPC management API (and so the SDK), submissions and object hooks keep the `embed_*` fields:

- **Reads** flatten the embedder into them (`Datasource.FlattenedEmbed()`, `EmbedFields(resolve)`); REST responses add `embedder_id` and `embedder_name`. `models.Datasource` JSON (object hooks, system events, submission version snapshots) carries the same keys through a custom `MarshalJSON`.
- **Writes** go through `EmbedderInput`. An explicit `embedder_id` wins (0 unlinks). Otherwise the legacy fields are merged onto the current embedder with the old rules (empty vendor/url/model keep the value, `"[redacted]"` keeps the key, `""` clears it) and:
  - nothing changed → same embedder, no write;
  - a linked embedder with only the model changed → the embedder linked to the same LLM with that model (found or created);
  - otherwise → a standalone embedder with exactly that configuration and a privacy score covering the datasource (found or created, named `"<vendor> · <model>"`).
  A shared embedder is never edited through a datasource (copy-on-write).
- Embedders created this way are **not** validated, matching the old fields (a datasource could name a vendor that cannot embed, or no model). They appear in the Embedders list to be fixed. `POST/PATCH /embedders` is strict.
- Plugin hooks that edit `embed_*` in a returned object move the datasource the same way (`applyHookEmbedEdits`).
- Submission approval and version rollback resolve payload `embed_*`/`embedder_id` keys instead of writing columns; a pre-Embedders snapshot rolls back to the matching embedder.

## Migration

`models.MigrateEmbedders`, run by `InitModels` after AutoMigrate:

- selects datasources with inline settings and no embedder (so a second run finds nothing);
- groups identical `(vendor, endpoint, key, model)` (keys are plain values or references, never ciphertext, so string equality is exact); a Vertex row without an endpoint takes `db_conn_string`/`db_conn_api_key`, as the old Vertex embedder did;
- creates one standalone embedder per group with the group's highest privacy score, links the datasources and clears their columns;
- runs in one transaction under `pg_advisory_xact_lock` on Postgres, so replicas starting together migrate once;
- does nothing on a database created without the columns.

## Rules

- **Model lock**: model, vendor and linked LLM cannot change while any datasource references the embedder (409, `EmbedderLockedError`). The datasource's vectors came from the current model; the fix is a new embedder, re-point, re-process. Endpoint, key, name, description and privacy stay editable. (Routers re-embed on reload, so they never lock.)
- **Privacy**: a datasource needs `embedder privacy ≥ datasource privacy`. Enforced on datasource writes with an explicit embedder (400), on embedder updates (409), and on LLM updates that lower the score of an LLM linked embedders use (409).
- **Delete**: refused while datasources (or routers) use the embedder (409 with the list). Deleting an LLM with linked embedders is refused (409); changing its vendor is refused when datasources embed through it, or to a vendor that cannot embed.
- **Dependents**: `GET /embedders/:id/dependents`; LLM dependents list `embedders`; secret dependents list embedders.

## API

| Method | Path | Permission |
|---|---|---|
| GET | `/embedders` (`search`, `sort`, `all`) | `embedders:read` |
| POST | `/embedders` | `embedders:write` |
| GET | `/embedders/:id` | `embedders:read` |
| PATCH | `/embedders/:id` | `embedders:write` |
| DELETE | `/embedders/:id` | `embedders:delete` |
| GET | `/embedders/:id/dependents` | `embedders:read` |
| GET | `/embedders/vendors` | `embedders:read` |

Responses redact the key (`api_key: "[redacted]"`, `has_api_key`); secret references are shown as they are. Linked embedders also carry `llm_name` and the inherited `vendor` and `privacy_score`.

RBAC resource `embedders` (group LLM management, actions CRUD). Built-in roles follow the catalogue defaults (Editor full, Viewer/Auditor read). Creating an embedder inline from the datasource or router form needs `embedders:write`.

Events: `system.embedder.created|updated|deleted` (payload redacted). They are config topics for edge sync and appear in the webhook topic list; embedders are a pending-change source (global).

## Admin UI

- **LLM management → Embedders** (`/admin/embedders`, `embedders:read`): list with the connection (LLM provider or API compatibility) and privacy level; detail page with a "Used by" section; add/edit form (`embedders:write`); delete through the dependents-aware confirmation, with the server's 409 shown when it is refused.
- `EmbedderFormFields` holds the fields for both the page and the dialog: a mode toggle (**Use an LLM provider** / **Standalone**; switching clears the other mode's fields), an LLM picker limited to vendors that embed, an **API compatibility** select from `/embedders/vendors` (vendor defaults for model and URL), endpoint (labelled *Project and location* for Vertex), key (secret-reference aware), model and privacy level. When datasources use the embedder the model and compatibility are disabled with the reason.
- **`EmbedderPicker`** is the reusable picker + inline creator (`EmbedderCreateDialog`). It lists embedders with their model, connection and privacy, flags one below the form's required privacy level, and offers **New embedder** only with `embedders:write`. Without `embedders:read` it shows the saved embedder's name read-only.
- The **data source form** uses the picker instead of the vendor/URL/key/model inputs and saves `embedder_id` (the flattened `embed_*` fields are not sent back). Data source list and detail show the embedder, linking to its page. The portal submission form keeps the legacy fields.

## Edges

Edges get no Embedder objects and the proto is unchanged. The snapshot flattens each datasource's embedder into `embed_vendor/url/api_key_encrypted/model` (a linked embedder resolves its LLM's connection); editing an embedder or its LLM changes those values, so the checksum moves and edges reload. The microgateway rebuilds an in-memory standalone embedder from the fields (`gateway_adapter.convertDatabaseDatasourceToModel`), falling back to the vector store fields for Vertex from an older hub.

## Follow-ups

- Drop the `embed_*` datasource columns after a rollback window.
- A RESTful datasource API that deprecates the `embed_*` fields.
- The Semantic Router switch-over (routers reference an embedder; the form uses `EmbedderPicker`) ships separately.
