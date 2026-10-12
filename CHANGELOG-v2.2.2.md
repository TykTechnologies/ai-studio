# Tyk AI Studio 2.2.2

**Status:** Released — `v2.2.2`
**Release date:** 2026-10-12
**Diff:** [`v2.2.1...v2.2.2`](https://github.com/TykTechnologies/ai-studio/compare/v2.2.1...v2.2.2)

2.2.2 is a security release for the microgateway. **Upgrade every microgateway, edges and standalone, as soon as you can.** AI Studio itself is unchanged from 2.2.1 and does not need upgrading.

> **Security advisory: unauthenticated admin access to the microgateway, leading to malicious plugin loading.**
> In 2.2.1 and earlier, anyone who could reach a microgateway's HTTP port could give themselves an admin token with one unauthenticated request (`POST /api/v1/auth/token`). With it they could load a malicious plugin of their own onto the gateway and attach it to its LLMs. A plugin runs with the gateway's trust: it sees and can change every prompt and response on the LLMs it is attached to, and as an auth plugin it can let any request through. Edges and standalone microgateways were both affected. AI Studio's own API and its embedded gateway were not.

2.2.2 closes this in two ways:

- **No microgateway issues tokens without credentials.** `POST /api/v1/auth/token` is removed in every mode.
- **Edges no longer serve a management API at all.** An edge gateway (`GATEWAY_MODE=edge`) answers `404` on every `/api/v1` path. Its LLMs, Apps, filters and plugins come only from AI Studio.

Read **What to check** below whether or not you have seen anything unusual, and **Breaking changes and upgrade notes** if you script against a microgateway's `/api/v1`.

---

## Security

- **`POST /api/v1/auth/token` issued admin tokens to anyone** ([#731](https://github.com/TykTechnologies/ai-studio/pull/731), [#732](https://github.com/TykTechnologies/ai-studio/pull/732)). The route needed no credentials and issued a token for any App with any scopes, `admin` included. That token opened the whole management API to a caller who could reach the microgateway's HTTP port:
  - **register plugins and attach them to LLMs** (`POST /api/v1/plugins`, `PUT /api/v1/llms/{id}/plugins`). A plugin served over gRPC from the attacker's own host needs no signature, and a plugin can also be a binary already on the gateway's host. OCI plugins must be signed unless `OCI_PLUGINS_REQUIRE_SIGNATURE` was set to `false`;
  - change LLMs (including their upstream endpoints), Apps, filters and prices;
  - read analytics, with request and response bodies where they are stored;
  - mint tokens for any App.

  The route is removed in every mode. Tokens are created by an admin (`POST /api/v1/tokens`) or with `microgateway -create-admin-token`.
- **Edge gateways serve no `/api/v1`** ([#731](https://github.com/TykTechnologies/ai-studio/pull/731), [#732](https://github.com/TykTechnologies/ai-studio/pull/732)). An edge is configured by AI Studio over gRPC, and its local management API wrote to the edge's copy of that configuration, so whatever it changed, including plugins, was served until the next configuration change from AI Studio or a restart. The API is no longer mounted, even for a valid admin token. The AI gateway endpoints, custom plugin endpoints under `/plugins/`, `/health`, `/ready` and metrics are unchanged.

### What to check

- **Plugins you did not install.** On each microgateway, compare its plugins with the ones you expect: `GET /api/v1/plugins` on a standalone gateway, or the `plugins` table in an edge's SQLite database (taken before upgrading, since an upgraded edge replaces it from AI Studio). Look for `grpc://` commands pointing at hosts you do not run and for `file://` commands you did not configure.
- **If you find one, treat the gateway as compromised.** The plugin could read and change the traffic on the LLMs it was attached to. Rotate the App credentials used through that gateway and the upstream provider keys of its LLMs, review the gateway host, and on an edge start it with a fresh data directory, which it rebuilds from AI Studio (analytics it had not yet delivered are lost).
- **Tokens.** On a standalone gateway, list the tokens (`GET /api/v1/tokens`) and revoke any you did not create, especially ones with the `admin` scope; such a token also authenticates gateway calls for its App. On an edge, gateway calls are checked with AI Studio, so a token minted on the edge could not authenticate them.
- **Other changes.** On a standalone gateway, review LLM endpoints, filters and prices. On an edge, upgrading restarts it, and on start it applies AI Studio's configuration in full, replacing changes made through the old API.
- **Exposure.** Keep microgateway ports reachable only by the clients that need the gateway.

## Breaking changes and upgrade notes

- **Edges answer `404` on every `/api/v1` path**, and the `mgw` CLI no longer works against an edge. Configure edges from AI Studio (**Edge Gateways**, **Push Configuration**).
- **Standalone microgateways keep their management API**, which still requires an admin token. Create tokens with `POST /api/v1/tokens` (admin token) or `microgateway -create-admin-token` instead of `POST /api/v1/auth/token`.
- **AI Studio does not need upgrading.** A 2.2.1 AI Studio works with 2.2.2 edges, and the config-sync protocol is unchanged.

## Submodules

No submodule changes from 2.2.1.
