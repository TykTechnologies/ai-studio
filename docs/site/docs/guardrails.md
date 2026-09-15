---
title: "Guardrails"
weight: 41
---

# Guardrails

A **guardrail** is a filter that runs a detection provider instead of a script. You choose a provider (the built-in pattern library, or an external classifier or NER service), the detectors to enable, and what to do on a hit: block, redact or log. No code.

Guardrails are a *kind* of filter, so everything that applies to filters applies to them: they attach to LLMs, chat rooms and tools, run on requests, responses (including streaming) and tool calls in both directions, on AI Studio and on every edge gateway, and every hit is a compliance event. The [Filters](/docs/filters) guide covers attachment and scopes; this page covers what is specific to guardrails.

Guardrails are an Enterprise feature. In the Community Edition a guardrail filter can be created and attached but is evaluated as a pass-through, exactly like a script filter.

## Creating a guardrail

In **Filters**, add a filter and choose **Guardrail** as the filter type. The form asks for:

| Field | Meaning |
|---|---|
| Provider | Which engine classifies the text. See [Providers](#providers). |
| On detection | `block` refuses the request (or stops the response); `redact` rewrites the matched text before it reaches the vendor; `log` lets it through. All three record a compliance event. |
| Detectors | What the provider looks for. For the built-in library these are categories; for external providers they are the provider's own detector names, with a threshold where the provider scores. |
| Messages to inspect | On LLM requests: the last user message, all user messages (default), system and user messages, or every message. Chat messages and tool calls always inspect the whole text. |
| If the provider fails | `closed` blocks (default on requests and tool calls), `open` lets the text through (default on responses). A timeout or error records a `guardrail.error` compliance event either way. |
| Timeout | Per provider call, default 2000 ms. |
| Streaming cadence | On response filters: re-check the accumulated response every N characters (default 250 for the built-in library, 1000 for remote providers), and once more when the stream completes, so a response shorter than the cadence is still checked. Remote providers are never called per chunk. A block mid-stream stops the stream; a block at completion ends it with an error event and is recorded, but chunks already sent have reached the client. |
| Block message | What the caller sees on a block. Findings are never included. |
| Connection | Endpoint, key and other settings for remote providers. Keys are `$SECRET/name` or `$ENV/NAME` references, resolved when the filter runs and when the configuration is pushed to edges. |

The **Test** panel runs the guardrail against sample input. For a remote provider this is a real call using the connection settings, so the panel is also the quickest way to check credentials.

### Redaction

`redact` rewrites the matched spans. Styles:

- **placeholder** (default): `[REDACTED:{{type}}]`, where `{{type}}` is the detector (`EMAIL`, `PAYMENT_CARD`, `AWS_ACCESS_KEY`) and `{{category}}` the category (`PII`, `SECRETS`).
- **mask**: the same number of `*` characters.
- **hash**: `[TYPE:1a2b3c4d]`, a short SHA-256 prefix, so the same value redacts to the same token.

Redaction applies to LLM requests, chat messages, tool arguments and tool results. LLM **responses are block-only**: a response guardrail set to `redact` records the finding and lets the response through, and the form says so.

### Compliance events and metrics

Every hit records one compliance event per detector, of type `guardrail.` followed by the detector name (`guardrail.secrets.aws_access_key`, `guardrail.prompt_attack`, `guardrail.pii.email`). The event carries the provider, detector, category, count, the highest score, the action taken, the message roles involved and the provider latency. It never carries the matched text. A provider failure records `guardrail.error`.

Blocks count towards `aistudio_policy_blocks_total` like any filter block. Two guardrail-specific series are exported:

| Metric | Labels | Meaning |
|---|---|---|
| `aistudio_guardrail_latency_seconds` | `provider`, `outcome` (`clean`, `flagged`, `error`), `action` | Provider call latency |
| `aistudio_guardrail_errors_total` | `provider`, `fail_mode` | Provider failures and the fail mode applied |

### Seeded defaults

A fresh Enterprise install seeds four guardrails using the built-in library, **unattached**, so nothing is enforced until an administrator attaches one:

- Credentials in prompts (block)
- Personal data before the vendor (redact)
- Prompt injection heuristics (log)
- Credential leakage in responses (block, response filter)

They are created by name and never overwritten; delete or rename them freely. Set `SKIP_FILTER_DEFAULTS=true` to skip seeding.

## Providers

`GET /api/v1/filters/guardrail-providers` describes every provider: its detectors, connection fields and whether it is available in this build.

### Built-in pattern library {#built-in}

Runs in-process with no network call. Patterns are validated where a regex alone would produce false positives: payment cards must pass Luhn, IBANs mod-97, US Social Security numbers must be in an issued range, UK National Insurance numbers follow the prefix rules, NHS numbers carry their check digit, and generic secrets must have the entropy of a secret. Credential patterns are derived from the gitleaks default ruleset (MIT).

Detectors are the four categories; individual patterns can be excluded by id.

| Category | Default action | Patterns (ids under the category prefix) |
|---|---|---|
| `secrets` | block | `aws_access_key`, `aws_secret_key`, `github_pat`, `github_fine_grained_pat`, `github_oauth`, `github_app_token`, `gitlab_pat`, `slack_bot_token`, `slack_user_token`, `slack_app_token`, `slack_webhook`, `stripe_key`, `openai_key`, `anthropic_key`, `google_api_key`, `gcp_service_account`, `azure_client_secret`, `azure_storage_key`, `private_key`, `jwt`, `twilio_key`, `sendgrid_key`, `huggingface_token`, `databricks_token`, `npm_token`, `pypi_token`, `mailchimp_key`, `mailgun_key`, `digitalocean_token`, `postman_key`, `telegram_bot_token`, `vault_token`, `age_secret_key`, `connection_string`, `basic_auth_url`, `bearer_token`, `generic_api_key` |
| `pii` | redact | `email`, `phone_international`, `phone_us`, `us_ssn`, `payment_card`, `iban`, `uk_nino`, `uk_nhs`, `ipv4`, `ipv6`, `mac_address`, `date_of_birth`, `passport`, `drivers_license`, `medical_record`, `street_address`, `eu_vat` |
| `injection` | log | `instruction_override`, `role_hijack`, `system_prompt_exfil`, `pseudo_tags`, `markdown_exfil`, `zero_width`, `base64_blob`, `indirect_marker` |
| `leak` | block (responses) | `system_prompt_disclosure`, `markdown_exfil` |

The injection category is a **heuristic**: it catches the explicit, well-known phrasings and encoding tricks and is a sensible baseline for air-gapped deployments and defence in depth. It is not a classifier. For real prompt-injection detection pair it with one of the classifier providers below.

The same library is available to scripts as `tyk.detect(text, categories)` and `tyk.redact(input_or_text, categories, placeholder)`; the script templates **Built-in library: block credentials** and **Built-in library: redact personal data** show both.

### HTTP classifier {#http}

Any classifier or NER service behind a documented JSON contract, for wrapping self-hosted models such as Llama Prompt Guard 2 or LLM Guard, or an in-house model.

Connection: `endpoint` (required), `api_key` (sent as `Authorization: Bearer`, optional), `auth_header` (an alternative header name for the key).

The gateway POSTs:

```json
{
  "direction": "request",
  "detectors": ["prompt_attack"],
  "redact": false,
  "segments": [{"index": 0, "role": "user", "text": "..."}],
  "meta": {"request_id": "...", "vendor": "openai", "model": "gpt-4"}
}
```

and expects:

```json
{
  "flagged": true,
  "findings": [
    {"detector": "prompt_attack", "category": "injection", "score": 0.98, "segment_index": 0, "span": {"start": 0, "end": 40}}
  ],
  "rewritten": {"0": "redacted text"}
}
```

`findings[].score` is compared with the detector's threshold when one is set (default 0.5). `rewritten` is optional and only used when `redact` was true. Detector names are whatever the service defines; the form lets you type them.

### Microsoft Presidio {#presidio}

Self-hosted named-entity PII detection. Detectors are Presidio entity types (`PERSON`, `EMAIL_ADDRESS`, `CREDIT_CARD`, `US_SSN`, `IBAN_CODE`, ...) with the 0..1 recogniser score as threshold (default 0.5). Connection: `analyzer_url` (required), `anonymizer_url` (optional: when set, the anonymizer produces the redacted text; otherwise redaction is applied from the analyzer's spans), `language` (default `en`).

### Lakera Guard {#lakera}

Lakera Guard v2. Detectors: `prompt_attack`, `pii` (or a subtype such as `pii/email`), `moderated_content` (or a subtype), `unknown_links`, `custom`. Which of these actually run is decided by the policy of the Lakera project. Connection: `api_key` (SaaS), `endpoint` (an EU or Asia host, or a self-hosted Guard, which needs no key), `project_id`. PII findings carry spans, so `redact` is available for them.

### Azure AI Content Safety {#azure-content-safety}

Detectors: `prompt_attack` (Prompt Shields) and the moderation categories `Hate`, `Sexual`, `SelfHarm`, `Violence`, each with a severity threshold 0..6 (default 4). Connection: `endpoint`, `api_key` (`Ocp-Apim-Subscription-Key`), optional `blocklist_names`. Text longer than 10,000 characters is split. Block or log only.

### Azure AI Language PII {#azure-pii}

Named-entity PII with server-side redaction. Detectors are Azure PII categories (`Person`, `PhoneNumber`, `Email`, `CreditCardNumber`, `USSocialSecurityNumber`, ...) or `all`, with a confidence threshold. `all` also covers `PersonType`, `Organization` and `DateTime`, which match ordinary prose (a job title such as "lighthouse keeper" is a `PersonType`), so name the categories explicitly for a blocking policy. Connection: `endpoint` (cloud or the Text PII container), `api_key`, `language`, optional `domain` (`phi`).

### Amazon Bedrock Guardrails {#bedrock}

`ApplyGuardrail` against a guardrail defined in Bedrock. It works with any model, not only Bedrock-hosted ones. Detectors select which assessments count: `content` (including prompt attacks), `topic`, `word`, `sensitive_information` (PII and regexes; anonymised entities come back as masked text, so `redact` is available), `grounding`. Connection: `guardrail_id`, `guardrail_version` (`DRAFT` or a number), `region`, and optionally `access_key_id` and `secret_access_key` (otherwise the gateway's ambient AWS credentials are used).

## Verifying a provider

The filter form's **Test** panel runs a guardrail against sample input with a real provider call. For a repeatable check of every provider with real credentials, the repository ships a credential-gated conformance suite: fill in the `VT_GUARD_*` section of `test-secrets/vendors.env` and run `make test-guardrails` (see `features/VendorConformance.md`, section 9). It probes each provider directly with benign, injection, PII and credential texts, then attaches them as filters to a real LLM route and drives the gateway.

## Edge gateways

Guardrail filters reach edges in the configuration snapshot like script filters do, with connection references resolved by the hub, since edges have no secret store. The same filter chain runs at the edge, so a guardrail behaves identically on both planes and its compliance events reach AI Studio on the analytics pulse.

## API

A guardrail is a filter with `kind: "guardrail"` and a `config`:

```json
{
  "data": {
    "type": "filter",
    "attributes": {
      "name": "Block credentials",
      "description": "Block API keys in prompts",
      "kind": "guardrail",
      "response_filter": false,
      "config": {
        "provider": "builtin",
        "detectors": [{"name": "secrets"}],
        "exclude": ["secrets.jwt"],
        "on_detect": "block",
        "scope": "all_user",
        "fail_mode": "closed",
        "timeout_ms": 2000,
        "block_message": "Request blocked by policy"
      }
    }
  }
}
```

The stored config is normalised: defaults are written out, and an invalid config (unknown provider or detector, `redact` on a provider that cannot rewrite, a missing required connection field) is rejected with HTTP 400 and the reason. `POST /api/v1/filters/test` accepts `kind`, `config` and `response_filter` alongside `input` to run a guardrail without saving it. One rule applies to the connection block: `$SECRET/` and `$ENV/` references are only resolved when the request also carries `filter_id` and the provider and connection settings are exactly those saved on that filter. An ad-hoc config with a reference, or a saved filter tested with a changed endpoint, is refused with HTTP 400 and nothing is sent. Otherwise anyone allowed to run a test could have a stored secret resolved and posted to a host of their choosing. Literal connection values need no saved filter.

Saving a filter is where that trust sits. Write permission on filters, like write permission on LLMs, lets a user point a stored `$SECRET/` reference at an endpoint of their choosing, so grant it as you would grant the ability to edit LLM connections. The secret value itself stays in the secret store and is only ever sent from the gateway; it is never returned by the API.

At run time a saved filter's config is parsed, normalised and its references resolved once, then reused for 30 seconds per filter version (ID and update time), so a request does not pay a secret-store read per attached guardrail. A rotated secret or an edited filter takes effect within that window; saving a filter changes its version and takes effect immediately.
