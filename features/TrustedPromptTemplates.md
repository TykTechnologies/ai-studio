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
| `app_id`, `name`, `version` | One app has a small number of templates (cap: 10 approved). A name is letters, digits, `.`, `_` and `-`; events name a template `name@version`. |
| `placement` | `role` (`system` or `user`), `message` (`first`, `last` or `any` message of that role in the request, default `any`) and `anchor` (`start`, `end` or `both`, default `both`). A template anchored at a side starts (or ends) with a constant there; text beyond an unanchored side is an implicit slot. |
| `parts` | An ordered list of `{const: "<exact text>"}` and `{slot: "<name>"}` items. Each constant is 32 bytes to 64 KiB of valid UTF-8 with no invisible characters (see Matching), and does not start with a combining mark. Two constants or two slots in a row are refused. |
| `status` | `proposed`, `approved` or `retired`. Records who approved it and when. Only `approved` templates are matched. |
| `expires_after_days` | The template lapses after this long with no matches (counted from its approval when it has never matched). Default 30. |

The `developer` role of the first draft is not offered yet: the guardrail segmenter reads an OpenAI `developer` message as user text, so a developer placement could not be told apart from a user one.

Each row is one version. A change to the parts or placement of a proposed version edits it in place; the same change to an approved or retired version creates the next version, `proposed`, and leaves the old one as it was. Approving a version retires the other approved versions of the same name, so one version of a name is trusted at a time. The table is `app_prompt_templates` (`models.AppPromptTemplate`; `models.PromptTemplate` is the unrelated chat prompt shortcut).

Constant text is stored as written; it is the app's own prompt and is not secret. Storing only an HMAC and a byte length, with no readable text, is a later option for tenants that want it.

**API:** under `/api/v1/apps/:id/prompt-templates`: `GET` (list, `?status=` filters), `POST` (propose), `GET|PATCH|DELETE /:template_id`, `POST /:template_id/approve` and `POST /:template_id/retire`. Listing needs `apps:read`; proposing, editing and retiring `apps:write`; deleting `apps:delete`. Approving needs a full administrator. Approving a template that is not proposed, or an eleventh one, is a `409`.

**Hint header:** callers may send `X-Tyk-Prompt-Template: <name>@<version>`. It selects which approved template to try first. A forged or stale hint gains nothing, because only approved bytes are ever exempt.

## Matching

The matcher is `guardrails/templates.MatchMessage`, a pure function. For each message whose role and position fit a template's placement:

- Every `const` part must appear **byte for byte, in the declared order**, at the anchored position. There is no Unicode normalisation, no case folding and no whitespace folding. A message that is not valid UTF-8 never matches.
- Slot boundaries come only from the constants matched around them, never from a delimiter such as `</instructions>` on its own. Slots may be empty.
- If a message can be parsed more than one way, the parse that scores more text wins. Every parse of one template exempts the same constants, so parses differ in what they score only when a copy of a constant cannot be exempt (below); among parses that score the same, the leftmost wins. Across templates, the match that exempts the fewest characters wins; ties go to the earlier template.
- A constant that contains zero-width, bidi, Unicode tag, variation-selector or other format characters is refused when it is stored, and never matched. A constant matched next to one, or followed by a combining mark that would join its last character, is matched but **not exempt**: it is scored.
- Each constant is exempt **once**, at the position the parse gives it. Template text that appears inside a slot (any 32-byte run of a constant) is a **mimicry signal**: it is scored and reported (`template.mimicry`), never exempt.
- A slot holding the placeholder text (`[app template]`), or a constant that occurs more than 64 times, makes the message not match (it is scored whole) and is reported as mimicry.
- If no template matches a message that some template's placement fits, the message is scored whole and a `template.miss` is recorded.

## Scan plan

Where it runs: `scripting/guardrail_templates.go`, called from the guardrail runner's `evaluate` for request filters. Studio registers the template source (`scripting.SetPromptTemplateSource`, backed by `services.PromptTemplateSource`, cached per app for 15 s). Nothing else registers one, so edge gateways take none of this path until templates are synced (bead .8).

Which messages are tried: the request's own `system` and `user` text that the filter scans (its scope applies first), one block per message. Tool results, tool calls, tool definitions, documents, detection-only views and a message whose text the vendor carries in several blocks are never tried; neither are responses, Studio chat tool results or tool calls. `first` and `last` count every message of the role in the request, in scope or not. The app is the request's `app_id`; a request with no app (Studio chat) is never matched.

When a template matches, each guardrail on the request receives the message as a **scan plan**: the original text with the exempt ranges marked.

- **Residual:** the slots in their original order, with each exempt constant replaced by a fixed placeholder (`[app template]`). This is what's scored. Every provider, including the built-in library, Lakera and Bedrock, scores the residual unchanged. The budgets, `max_scan_chars`, the unscanned rules and the fail mode apply to the residual as to any text.
- **Exemption cap:** at most 60% of a message's characters can be exempt, and at most 16k characters. Constants are taken in order; one that would pass either limit is scored.
- **Full-text backstop** (`template_backstop`, per filter: `off`, `log` or `enforce`, default `log`): the unmodified message is scored as well.
  - A provider that reads templates (`Capabilities.Templates`) and is sent the segment whole gets `full_text` in the same call and marks what it finds there `"backstop": true`. The runner honours the mark only on segments it sent with a template, so a provider cannot turn an ordinary finding into a logged one.
  - Every other provider, and a template-aware one whose segment was split into pieces, gets a second classification of the full text (no rewrite).
  - `log`: backstop findings are recorded as compliance events with action `log` and `template_backstop: log`, and change nothing. A backstop that fails is ignored.
  - `enforce`: a backstop finding the residual also made (same detector) is already acted on; any other is the filter's own finding. Under `redact` it blocks, since a redaction of the residual does not reach it. A backstop that fails, or text it could not scan, follows the fail mode and unscanned rules like the main scan.
- **Redaction:** a provider's rewrite of the residual is mapped back onto the message: the text between placeholders is the rewritten slots, and each placeholder becomes the constant it stood for. A rewrite that does not keep exactly one placeholder per exempt constant cannot be placed, and the segment's redaction falls to a block. Finding spans map from residual to original offsets.

**HTTP classifier contract (additive):** a segment from a template match carries:

```json
{"index": 0, "role": "user", "text": "<residual>",
 "template": {"id": "pr-review", "version": 3, "full_text": "<original>",
              "exempt": [{"start": 0, "end": 812}, {"start": 1904, "end": 2310}],
              "backstop": "log"}}
```

`exempt` are byte offsets into the UTF-8 `full_text`, like finding spans. `backstop` is the filter's `template_backstop`. A service that ignores `template` scores the residual. A template-aware service, such as the Wolf Defender sidecar, can do its own windowing and, unless `backstop` is `off`, score `full_text` too, returning those findings with `"backstop": true`. The backstop then costs no second call. The gateway honours `backstop: true` only on segments it sent with a `template`. `template` is sent only with a segment sent whole; a segment split into pieces gets the second-call backstop. In code: `guardrails.Segment.Template` (`SegmentTemplate`), `guardrails.Finding.Backstop`, and `guardrails.Capabilities.Templates`, which a provider declares to be sent templates.

The Enterprise HTTP provider maps segments through its own wire type, so it sends `template` only once it copies `Segment.Template` onto the wire and declares `Capabilities.Templates` (a change in the enterprise module, not part of this milestone). Until then it is treated like any other provider: it scores the residual and gets the backstop as a second call.

**Verdict cache:** the key of a matched message covers the residual plus the template id, version, the backstop mode and the full text, so a template change re-scores the text and a cached residual is never answered for an unmatched message. Keys of every other segment are unchanged.

**No templates, no change:** with no source registered, no app, or no approved template whose placement fits a message, the request is screened exactly as before (`TestTemplates_NoneRegisteredIsByteIdentical`).

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
