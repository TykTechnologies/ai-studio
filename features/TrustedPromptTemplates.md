# Trusted prompt templates (design draft)

Status: draft for review, 2026-10-30. Not implemented.

## Problem

Request guardrails classify a whole message. An app that wraps third-party content in its own instructions sends both in one `user` message, for example a code-review bot's "You are reviewing this PR… follow these rules…" around a diff and PR comments. A prompt-injection classifier reads the app's own instructions as instructions aimed at the model and flags them. Today the operator can only exempt the whole message, which also exempts the third-party content, or accept the false positives.

The feature lets an operator approve an app's own template once. Guardrails then score only the parts that vary, the **slots**.

This is a false-positive control, not a security boundary. It must never make the gateway miss an attack it catches today by scoring the whole prompt, and the docs say so.

## Principles

1. **Trust comes from an operator approving exact bytes**, never from the request, from how often text recurs, or from a delimiter. A caller's hint only narrows the search.
2. **Fail closed to today's behaviour.** Anything short of an exact match scores the whole text, as if no template were registered.
3. **Untrusted channels are never exempt.** Tool results, documents and tool calls are always scored, however often they recur.
4. **The guardrail sees what the model reads.** The parts that vary are scored in their original order, with a placeholder where the template was, so nothing the model reads goes unscored except the approved bytes.

## Model

A **prompt template** is an app-level resource. Trust belongs to the calling app, while filters attach to LLMs.

| Field | Meaning |
|---|---|
| `app_id`, `name`, `version` | One app has a small number of templates (cap: 10 approved). |
| `placement` | Role (`system`, `user`, `developer`), message (first, last, any) and anchoring (starts the message, ends it, or both). |
| `parts` | An ordered list of `{const: "<exact text>"}` and `{slot: "<name>"}` items. Each constant is at least 32 bytes long. |
| `status` | `proposed`, `approved` or `retired`. Records who approved it and when. |
| `expires_after` | The template lapses after this long with no matches. Default 30 days. |

Constant text is stored as written; it is the app's own prompt and is not secret. Storing only an HMAC and a byte length, with no readable text, is an open question for tenants that want it.

**API:** CRUD under `/api/v1/apps/:id/prompt-templates`, plus approve and retire actions. Only an admin can approve; an app owner can propose.

**Hint header:** callers may send `X-Tyk-Prompt-Template: <name>@<version>`. It selects which approved template to try first. A forged or stale hint gains nothing, because only approved bytes are ever exempt.

## Matching

For each message whose role and position fit a template's placement:

- Every `const` part must appear **byte for byte, in the declared order**, at the anchored position. There is no Unicode normalisation, no case folding and no whitespace folding.
- Slot boundaries come only from the constants matched around them, never from a delimiter such as `</instructions>` on its own. If a message can be parsed more than one way, the parse that scores more text wins.
- A constant that contains, or sits next to, zero-width, bidi, Unicode tag or variation-selector characters is not exempt.
- Each constant is exempt **once**, at its anchored position. Template text that appears again inside a slot is a **mimicry signal**: it is scored and reported (`template.mimicry`), never exempt.
- If no template matches, the message is scored whole and a `template.miss` counter is recorded per app and template.

## Scan plan

When a template matches, each guardrail on the request receives the message as a **scan plan**: the original text with the exempt ranges marked.

- **Residual:** the slots in their original order, with each exempt constant replaced by a fixed placeholder (`[app template]`). This is what's scored. Every provider, including the built-in library, Lakera and Bedrock, scores the residual unchanged.
- **Exemption cap:** at most 60% of a message's characters can be exempt, and at most 16k characters. Anything beyond the cap is scored.
- **Full-text backstop:** the unmodified message is scored as well. Its action is `log` by default; the operator can set it to the filter's action once its false-positive rate is known.
- **Redaction:** offsets map from the residual back to the original. A span that falls on template text cannot be redacted, which is impossible by construction since that text is never scored.

**HTTP classifier contract (additive):** a segment from a template match carries:

```json
{"index": 0, "role": "user", "text": "<residual>",
 "template": {"id": "pr-review", "version": 3, "full_text": "<original>",
              "exempt": [{"start": 0, "end": 812}, {"start": 1904, "end": 2310}]}}
```

A service that ignores `template` scores the residual. A template-aware service, such as the Wolf Defender sidecar, can do its own windowing and backstop over `full_text`. The backstop then costs no second call.

**Verdict cache:** the cache key covers the residual plus the template id and version, so a template change re-scores the text.

## Learned suggestions (later phase)

An optional miner proposes templates from traffic. It never approves them. A proposed template is eligible only when all of these hold:

- it appears at a stable position outside any untrusted channel;
- it appears in many requests with **distinct** slot content, spread over days rather than one burst;
- it never appears inside a slot or a tool result.

The approval screen shows:

- the exact bytes, with invisible characters made visible;
- where the text appears, and when it was first and last seen;
- how many distinct requests contained it;
- a diff against the previous version, with a warning when a change is small (a small edit is where an injection fits).

## Operations

- **Compliance events:** carry `template_id@version`, or `template.miss` / `template.mimicry`.
- **Metrics:** exempt share of characters per app; template hit and miss rate; backstop-only detections.
- **Edge gateways:** approved templates go in the configuration snapshot and its checksum. Matching runs where the guardrail runs.
- **Expiry and changes:** a template that hasn't matched for `expires_after` drops to `proposed`. Changing a constant creates a new version that needs approval again.

## Evaluation (acceptance gates)

1. **False positives by length:** on replayed app traffic, bucketed by window count, the false-positive target holds in every bucket.
2. **Injections into real benign slots:** known injections planted at the start, middle and end of slots, across window boundaries and next to template constants, are caught at least as often as by full-prompt scoring.
3. **Adaptive attack against the full pipeline**, by an attacker who knows the template: template mimicry, fake boundaries, near-template edits, Unicode smuggling, payloads split across slots, and references back to template rules. Attack success is no higher than with full-prompt scoring under the same aggregator.
4. **Poisoning:** one paragraph repeated across many requests and accounts never becomes an exemption without approval.

## Out of scope

- The aggregation fix for long inputs (same-window agreement, thresholds that depend on window count). It belongs in the classifier and ships separately, first.
- Per-request inline trust markers (Bedrock-style tags). They may come later, and only to mark *untrusted* spans.
- Exempting `system` messages by role alone.

## Decisions (proposed, pending Martin's confirmation)

These settle the open questions of the first draft. They are proposals until confirmed on midsommar-clean-x5f5.1.

1. **Edition: Community (core).** The model, API, matcher and scan plan are core; nothing lives in the enterprise module. Guardrail providers themselves remain Enterprise, so in the Community Edition templates can be stored and approved but have no effect until a provider runs.
2. **Storage: readable constant text.** The approval screen and edges need the bytes. Storing only an HMAC and a byte length is a later, opt-in option for tenants that want it.
3. **Backstop: per filter, `template_backstop: off | log | enforce`, default `log`.**
   - A provider that reads templates in-segment (the HTTP classifier, see the contract below) receives the full text as `template.full_text` and does its own backstop in the same call: no second call. Its backstop findings come back marked `"backstop": true`.
   - Every other provider gets a second classification of the full text. Its findings are logged (`log`) or treated as the filter's own findings (`enforce`).
