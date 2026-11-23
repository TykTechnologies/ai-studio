# Guardrail second opinion (LLM judge)

Status: implemented 2026-11-03 (midsommar-clean-9u1j.11 config/client/prompt, .12 record-only runtime, .13 UI, .14 soften mode; security fixes .45 coverage, .46 privacy, .50 budget, .57/.61 injection findings only). Implementation detail per bead is in [Filters.md](Filters.md); user docs in `docs/site/docs/guardrails.md` ("Second opinion (LLM judge)"). Epic: midsommar-clean-9u1j.

## What it is

A guardrail filter can ask a gateway-managed LLM (the judge) whether a segment it blocks or quarantines is in fact benign for the app that sent it (a code review quoting injection test vectors, a security tool, a support desk quoting phishing mail). The judge reads attacker-controlled text, so it may only ever soften a verdict, never below the app's capability floor, and any failure keeps the filter's verdict.

## Config

`guardrails.Config.Judge` (`guardrails/judge.go`): `mode` `off` (or absent) | `record` | `soften`; `llm_id`; `model` (empty: the LLM's default model); `timeout_ms` (default 3000, at most 30000). Shadow filters may only `record` (and, logging only, never have anything to judge). Saving checks (`services.checkJudge`) that the judge LLM exists, is active, serves chat, has a privacy score at least that of every LLM the filter is attached to, and reaches every edge namespace the filter runs in. The edge snapshot drops the judge from a filter's edge config when it is scored below an LLM the filter guards there (edges get no LLM privacy scores).

## What is judged

Segments the filter blocks or quarantines on a detector's finding, at most 4 per request. Never judged (the event records why, `verdict: none` with `error`):

- `category`: the segment has a finding that is not an injection finding. The judge is asked whether text is an attack, which means nothing for a secret, personal data, a leak, hateful or violent text, a denied topic or a blocklisted word ("not an attack" is right there and says nothing about that policy), so judging them would downgrade those blocks. Judged only when every finding is category `injection`, or `other`/none from the HTTP classifier (also over a Unix socket; Wolf Defender reports "other"), and never one whose detector names a data category (`judgeableFinding`, midsommar-clean-9u1j.57, .61).
  The judge also sees 'other' or uncategorised findings from HTTP classifiers, so turn it on only for filters whose HTTP classifier detects prompt injection (such as Wolf Defender); a classifier that reports moderation or PII as 'other' would have those judged.
- `over_segment_cap`, `privacy` (judge scored below the request's LLM, or a request naming no LLM: Studio chat is never judged, as exceptions do not apply there either), `over_budget` (the judge LLM's own budget is spent), `judge_unavailable`, `dropped` (background queue full).
- A segment blocked only because it was not scanned.

## Models and output limit

Any active chat LLM can be the judge. The call asks for one small JSON object, so its output limit is `JudgeMaxTokens` (64). A model that reasons before it answers has its reasoning tokens counted against the same limit by its vendor and, with 64, often returned no answer at all (2.3.0-rc4: no verdict on 8 of 20 flags with `gpt-5.6-luna`, most calls ending at 63 or 64 output tokens; 6bg5.26). `JudgeTokenLimit` therefore sends `JudgeReasoningMaxTokens` (2048) for the model families that reason by default (`judgeReasoningModel`): OpenAI `o1`, `o3`, `o4` and `gpt-5*`, and Google `gemini-2.5*` and `gemini-3*`. Anthropic models think only on request and the judge never asks, so they keep 64, as does everything else. No reasoning effort is sent: the accepted values differ per model, and a refused value would be a failed call. The visible answer stays bounded for every model: the parser reads at most `judgeMaxAnswerBytes` (512) and refuses anything but the closed JSON shape, and no opinion never changes a verdict. Only parsed opinions are cached, so the change needs no new cache key. A reasoning judge costs more per flag (up to 2048 output tokens) and is slower: check the judge timeout (default 3000 ms) when choosing one.

## Prompt and answer

Built in and versioned (`JudgePromptVersion`, part of the cache key). Facts computed by the gateway (app name and owner-written description, one line each and bounded; where the text is; detector; declared tool names; whether the finding sits in a JSON string, a diff hunk or a code block) come before the flagged text: at most 4000 characters around the finding, every whitespace run replaced by `ˆ` (datamarking), fenced by a random per-call boundary. The answer must be exactly `{"verdict":"benign|suspicious|attack","reason":"<enum>"}` (closed reasons per verdict); anything else is no opinion. Since `guardrail-judge/2` the rules also say to use only those two keys and not to explain (midsommar-clean-9u1j.63): without it Claude Sonnet 5.5 added keys or wrote a sentence as the reason on about a third of flags, which the parser refuses and the 64-token answer limit cuts off. Prompt wording moves verdicts too: an extra "the reason is exactly one of the listed words" made Sonnet 5.5 call a role-play attack benign every time, so a prompt change is evaluated on attacks as well as false positives before it ships.

`coverage` is `full` only when the window holds the whole segment or every finding's span (computed per request, also on cache hits). Classifiers that report no span (Wolf before 0.3.0) in text longer than the window give `partial`: recorded, never softening. An HTTP classifier's span is kept by the scan planner since midsommar-clean-9u1j.65 (before, every remote span was dropped as piece-relative): each piece records its byte offset in its segment (`piece.sliced`, `piece.at`; split pieces of invalid UTF-8 are no slice), `piece.segmentSpan` moves a span that lies in the piece onto the segment, and rescanned seams of rewritten text keep none. A template residual's span is then mapped onto the message by `templatePlan.restore`; backstop findings still lose theirs there.

## Record only

Asked on the shadow pool after the request has gone on with its verdict. One `guardrail.judge` event per segment, the opinion under the reserved `metadata.judge` (scripts cannot forge it). Opinions are cached 10 minutes per app, judge LLM version, model, prompt version and judged text. Each call is one `LLMChatRecord` with interaction type `guardrail_judge`, the judge LLM and no app: it counts toward the judge LLM's budget, never the judged app's. Filter Events sums up "judge said benign on N of M flags" per filter; those figures are only as trustworthy as the gateways (edges included) that reported them.

## Soften

Asked inline, before the verdict is applied, only for segments the app's capability floor lets go softer (an acting app's tool results, calls and definitions, and response text, never). A parsed benign opinion with full coverage gives the segment the softest action the floor allows; anything else keeps the verdict. The request waits at most the judge's timeout. Events record `judge: {verdict, softened_from}`, and each softening raises `guardrail.outcome.judge_softened` with the judge's opinion (features/Filters.md, outcome events), so webhooks and plugins hear what the judge let through. The editor offers Soften once the filter's judge has recorded at least 20 opinions in 30 days: advisory, the server's guarantees are the floor, full coverage, benign-only and the never-judged rules above.
