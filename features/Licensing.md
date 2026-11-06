# Licensing (Enterprise Edition)

Community Edition needs no licence: every Community feature is available
and nothing is limited. This document describes the Enterprise Edition.

## What a licence is

An Enterprise licence is a signed token (a JWT signed with Tyk's RSA key)
that AI Studio reads from `TYK_AI_LICENSE`, or from a host embedding Studio
(`licensing.Config.LicenseSource`). It carries:

- an expiry date;
- the products it licenses. AI Studio accepts only a licence that names the
  AI Studio product; a correctly signed Tyk licence for another product is
  refused with "license does not include the Tyk AI Studio SKU";
- for AI Studio, its **features** (for example the portal, chat, the
  gateway, roles and permissions) and its **limits**.

Licences are issued by Tyk. The enterprise module documents the claim
format and how to issue one (`enterprise/docs/LicenseLimits.md`,
`enterprise/cmd/license-gen`).

## Limits

A licence limits four things across the whole deployment:

| Limit | What uses it |
|-------|--------------|
| Edge namespaces | each edge namespace other than the default one (see [Edge Gateways](../docs/site/docs/edge-gateways.md#licensed-namespaces-enterprise)) |
| Users with admin access | each enabled user who can open the admin area, whatever their role |
| Users with portal access | each enabled user with portal access |
| Apps | each App |

Each limit is a number or unlimited. Reaching a limit stops new use only:
the change that would go over it is refused with HTTP 409 and the code
`QUOTA_EXCEEDED` (gRPC: `ResourceExhausted`), and a message saying what to
remove or whom to ask. Nothing that exists is revoked, so after a licence
with lower limits is loaded, usage above them stays until it is reduced.

Single sign-on and host-managed sign-in never fail because of a seat limit:
the user signs in without the access that would go over it, and the Owners
are notified.

Enterprise Edition also has a per-user **App quota** for self-service App
creation (portal and plugin SDK), set globally with `DEFAULT_APP_QUOTA` and
per team. It is not part of the licence. See the admin guide,
[License and Quotas](../docs/site/docs/license.md).

## Validation

- **At start:** an invalid, expired or missing licence fails start-up
  (`Start` returns an error; the standalone binary exits).
- **Periodically:** every `LICENSE_VALIDITY_CHECK_PERIOD` (default 24h) the
  licence is read again and checked. A failure calls
  `licensing.Config.OnInvalid`, or exits the process when none is set. A
  host can reload a renewed licence at once (`studio.ReloadLicense`); a bad
  one is refused and the licence held stays.
- The limits are logged once the licence is accepted.

The Enterprise microgateway verifies its `TYK_AI_LICENSE` the same way
(signature, expiry and the AI Studio product). It does not enforce limits.

## How it fits together

- `services/licensing` is the public licence interface (validity, features,
  days left, telemetry). Community Edition's implementation is always
  valid.
- `services/entitlements` is the hook through which limits are enforced:
  App creation, user, team and role changes, edge namespace admission, the
  License page and near-limit notices call it. Community Edition's stub
  allows everything, licenses every namespace and has no License page
  (402).
- The enforcement and the licence format live in the private enterprise
  module (`enterprise/licensing/verify`, `enterprise/features/licensing`,
  `enterprise/features/entitlements`). Core has no limit names or counting
  rules.

## Telemetry

Enterprise Edition sends anonymised usage counts (LLMs, Apps, users, chats
and HTTP interactions) to Tyk unless `LICENSE_DISABLE_TELEMETRY=true`. The
licence key is hashed before it is sent. One report per deployment: only
the leader replica reports.

## Configuration

```bash
TYK_AI_LICENSE=eyJhbGciOiJSUzI1NiIs...     # required in Enterprise Edition

LICENSE_VALIDITY_CHECK_PERIOD=24h          # how often the licence is re-checked
LICENSE_DISABLE_TELEMETRY=false            # true disables telemetry
LICENSE_TELEMETRY_URL=...                  # telemetry endpoint
LICENSE_TELEMETRY_PERIOD=1h                # how often telemetry is sent
LICENSE_TELEMETRY_CONCURRENCY=20           # concurrent telemetry requests
DEFAULT_APP_QUOTA=1                        # self-service Apps per user ("*" unlimited)
```

Community Edition ignores all of these.
