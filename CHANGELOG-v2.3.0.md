# Tyk AI Studio 2.3.0

**Status:** Unreleased

---

## Highlights

- **Claude Code model discovery on the Anthropic → Bedrock bridge.** `GET /anthropic/{slug}/v1/models` lists the Bedrock model a connection really serves, so Claude Code's `/model` picker shows it as a "From gateway" entry (`<LLM name> — <model id>`) once a developer sets `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1`. It uses the same auth and App access check as `/v1/messages`, lists nothing when the default model is missing or refused by the allowed models, and calls no model, so it spends no budget. Needs Claude Code v2.1.223 or later for region-prefixed model ids; application inference profile ARNs are not shown. (TAS-34)
