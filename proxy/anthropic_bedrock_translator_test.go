package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- wire -> vendor mapping ---

func TestAnthropicMessagesToVendor_TextToolUseToolResult(t *testing.T) {
	req := &AnthropicMessagesRequest{
		System: json.RawMessage(`"You are helpful."`),
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "assistant", Content: json.RawMessage(`[
				{"type":"text","text":"let me check"},
				{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"NYC"}}
			]`)},
			{Role: "user", Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"toolu_1","content":"sunny","is_error":false}
			]`)},
		},
	}

	msgs, system, err := anthropicMessagesToVendor(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(system) != 1 || system[0].Text != "You are helpful." {
		t.Fatalf("system not mapped: %+v", system)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// assistant turn: text + tool_use
	asst := msgs[1]
	if asst.Role != "assistant" || len(asst.Blocks) != 2 {
		t.Fatalf("assistant blocks wrong: %+v", asst)
	}
	if asst.Blocks[0].Type != "text" || asst.Blocks[0].Text != "let me check" {
		t.Errorf("text block wrong: %+v", asst.Blocks[0])
	}
	tu := asst.Blocks[1]
	if tu.Type != "tool_use" || tu.ToolUseID != "toolu_1" || tu.Name != "get_weather" {
		t.Errorf("tool_use block wrong: %+v", tu)
	}
	if tu.Input["city"] != "NYC" {
		t.Errorf("tool_use input not decoded: %+v", tu.Input)
	}

	// user tool_result turn
	tr := msgs[2].Blocks[0]
	if tr.Type != "tool_result" || tr.ToolUseID != "toolu_1" || tr.ResultText != "sunny" || tr.IsError {
		t.Errorf("tool_result block wrong: %+v", tr)
	}
}

func TestAnthropicMessagesToConverse_CachePointInsertion(t *testing.T) {
	msgs := []bedrockVendor.AnthropicMsg{
		{Role: "user", Blocks: []bedrockVendor.AnthropicBlock{
			{Type: "text", Text: "big context", CachePoint: true},
		}},
	}
	system := []bedrockVendor.AnthropicSystemBlock{
		{Text: "sys prompt", CachePoint: true},
	}

	converseMsgs, systemBlocks := bedrockVendor.AnthropicMessagesToConverse(msgs, system)

	// user message: text block followed by a cache point => 2 content blocks
	if len(converseMsgs) != 1 || len(converseMsgs[0].Content) != 2 {
		t.Fatalf("expected text + cachepoint, got %+v", converseMsgs)
	}
	if _, ok := converseMsgs[0].Content[0].(*types.ContentBlockMemberText); !ok {
		t.Errorf("expected first block text, got %T", converseMsgs[0].Content[0])
	}
	if _, ok := converseMsgs[0].Content[1].(*types.ContentBlockMemberCachePoint); !ok {
		t.Errorf("expected second block cachepoint, got %T", converseMsgs[0].Content[1])
	}

	// system: text block followed by a cache point => 2 system blocks
	if len(systemBlocks) != 2 {
		t.Fatalf("expected system text + cachepoint, got %d", len(systemBlocks))
	}
	if _, ok := systemBlocks[1].(*types.SystemContentBlockMemberCachePoint); !ok {
		t.Errorf("expected system cachepoint, got %T", systemBlocks[1])
	}
}

func TestAnthropicMessagesToConverse_SkipsEmptyText(t *testing.T) {
	msgs := []bedrockVendor.AnthropicMsg{
		{Role: "assistant", Blocks: []bedrockVendor.AnthropicBlock{
			{Type: "text", Text: ""},
			{Type: "tool_use", ToolUseID: "t1", Name: "f", Input: map[string]any{"a": 1}},
		}},
	}
	converseMsgs, _ := bedrockVendor.AnthropicMessagesToConverse(msgs, nil)
	if len(converseMsgs) != 1 || len(converseMsgs[0].Content) != 1 {
		t.Fatalf("expected empty text skipped, got %+v", converseMsgs)
	}
	if _, ok := converseMsgs[0].Content[0].(*types.ContentBlockMemberToolUse); !ok {
		t.Errorf("expected tool_use block, got %T", converseMsgs[0].Content[0])
	}
}

func TestAnthropicToolsToToolConfig_ChoiceAndCachePoint(t *testing.T) {
	tools := []bedrockVendor.AnthropicToolSpec{
		{Name: "get_weather", Description: "d", InputSchema: map[string]any{"type": "object"}, CachePoint: true},
	}
	choice := &bedrockVendor.AnthropicToolChoiceSpec{Type: "tool", Name: "get_weather"}

	cfg := bedrockVendor.AnthropicToolsToToolConfig(tools, choice)
	if cfg == nil {
		t.Fatal("expected non-nil tool config")
	}
	// tool spec + cache point
	if len(cfg.Tools) != 2 {
		t.Fatalf("expected toolspec + cachepoint, got %d", len(cfg.Tools))
	}
	if _, ok := cfg.Tools[1].(*types.ToolMemberCachePoint); !ok {
		t.Errorf("expected tool cachepoint, got %T", cfg.Tools[1])
	}
	tc, ok := cfg.ToolChoice.(*types.ToolChoiceMemberTool)
	if !ok {
		t.Fatalf("expected specific tool choice, got %T", cfg.ToolChoice)
	}
	if aws.ToString(tc.Value.Name) != "get_weather" {
		t.Errorf("tool choice name wrong: %s", aws.ToString(tc.Value.Name))
	}

	if bedrockVendor.AnthropicToolsToToolConfig(nil, nil) != nil {
		t.Error("expected nil config for no tools")
	}
}

func TestConvertConverseStopReasonAnthropic(t *testing.T) {
	cases := map[types.StopReason]string{
		types.StopReasonEndTurn:      "end_turn",
		types.StopReasonToolUse:      "tool_use",
		types.StopReasonMaxTokens:    "max_tokens",
		types.StopReasonStopSequence: "stop_sequence",
	}
	for in, want := range cases {
		if got := bedrockVendor.ConvertConverseStopReasonAnthropic(in); got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
	}
}

// --- response mapping ---

func TestConverseOutputToAnthropicResponse(t *testing.T) {
	output := makeConverseOutput(
		[]types.ContentBlock{
			&types.ContentBlockMemberText{Value: "hi there"},
			&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
				ToolUseId: aws.String("toolu_9"),
				Name:      aws.String("do_thing"),
				Input:     lazyDoc(map[string]any{"x": 1}),
			}},
		},
		types.StopReasonToolUse, 11, 22, 33, 44,
	)

	resp := converseOutputToAnthropicResponse(output, "claude-x")
	if resp.Type != "message" || resp.Role != "assistant" || resp.Model != "claude-x" {
		t.Fatalf("response envelope wrong: %+v", resp)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("stop_reason: %s", resp.StopReason)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 22 {
		t.Errorf("usage wrong: %+v", resp.Usage)
	}
	if resp.Usage.CacheCreationInputTokens != 33 || resp.Usage.CacheReadInputTokens != 44 {
		t.Errorf("cache usage wrong: got creation=%d read=%d, want 33/44",
			resp.Usage.CacheCreationInputTokens, resp.Usage.CacheReadInputTokens)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" || resp.Content[0].Text != "hi there" {
		t.Errorf("text block wrong: %+v", resp.Content[0])
	}
	tu := resp.Content[1]
	if tu.Type != "tool_use" || tu.ID != "toolu_9" || tu.Name != "do_thing" {
		t.Errorf("tool_use block wrong: %+v", tu)
	}
	var input map[string]any
	if err := json.Unmarshal(tu.Input, &input); err != nil || input["x"] == nil {
		t.Errorf("tool_use input not marshalled: %s (%v)", tu.Input, err)
	}
}

// --- streaming SSE sequence ---

func TestAnthropicStream_TextThenToolUseSequence(t *testing.T) {
	rec := newSSERecorder()
	st := &anthropicStreamState{
		w:         rec,
		flusher:   rec,
		msgID:     "msg_test",
		echoModel: "claude-x",
		started:   map[int32]bool{},
		stopped:   map[int32]bool{},
	}

	events := []types.ConverseStreamOutput{
		&types.ConverseStreamOutputMemberMessageStart{Value: types.MessageStartEvent{Role: types.ConversationRoleAssistant}},
		// text block at index 0
		&types.ConverseStreamOutputMemberContentBlockDelta{Value: types.ContentBlockDeltaEvent{
			ContentBlockIndex: aws.Int32(0),
			Delta:             &types.ContentBlockDeltaMemberText{Value: "Hello"},
		}},
		&types.ConverseStreamOutputMemberContentBlockStop{Value: types.ContentBlockStopEvent{ContentBlockIndex: aws.Int32(0)}},
		// tool_use block at index 1
		&types.ConverseStreamOutputMemberContentBlockStart{Value: types.ContentBlockStartEvent{
			ContentBlockIndex: aws.Int32(1),
			Start: &types.ContentBlockStartMemberToolUse{Value: types.ToolUseBlockStart{
				ToolUseId: aws.String("toolu_1"), Name: aws.String("get_weather"),
			}},
		}},
		&types.ConverseStreamOutputMemberContentBlockDelta{Value: types.ContentBlockDeltaEvent{
			ContentBlockIndex: aws.Int32(1),
			Delta:             &types.ContentBlockDeltaMemberToolUse{Value: types.ToolUseBlockDelta{Input: aws.String(`{"city":`)}},
		}},
		&types.ConverseStreamOutputMemberContentBlockDelta{Value: types.ContentBlockDeltaEvent{
			ContentBlockIndex: aws.Int32(1),
			Delta:             &types.ContentBlockDeltaMemberToolUse{Value: types.ToolUseBlockDelta{Input: aws.String(`"NYC"}`)}},
		}},
		&types.ConverseStreamOutputMemberContentBlockStop{Value: types.ContentBlockStopEvent{ContentBlockIndex: aws.Int32(1)}},
		&types.ConverseStreamOutputMemberMessageStop{Value: types.MessageStopEvent{StopReason: types.StopReasonToolUse}},
		&types.ConverseStreamOutputMemberMetadata{Value: types.ConverseStreamMetadataEvent{
			Usage: &types.TokenUsage{
				InputTokens: aws.Int32(10), OutputTokens: aws.Int32(5),
				CacheWriteInputTokens: aws.Int32(70), CacheReadInputTokens: aws.Int32(80),
			},
		}},
	}
	for _, e := range events {
		st.handleEvent(e)
	}
	st.finish()

	types_ := rec.eventTypes()
	want := []string{
		"message_start", "ping",
		"content_block_start", "content_block_delta", "content_block_stop", // text
		"content_block_start", "content_block_delta", "content_block_delta", "content_block_stop", // tool_use
		"message_delta", "message_stop",
	}
	if len(types_) != len(want) {
		t.Fatalf("event count: got %v (%d) want %v (%d)", types_, len(types_), want, len(want))
	}
	for i := range want {
		if types_[i] != want[i] {
			t.Errorf("event %d: got %s want %s", i, types_[i], want[i])
		}
	}

	// The tool_use partial_json fragments must be forwarded verbatim.
	got := rec.dataFor("content_block_delta")
	joined := ""
	for _, d := range got {
		if d["delta"].(map[string]any)["type"] == "input_json_delta" {
			joined += d["delta"].(map[string]any)["partial_json"].(string)
		}
	}
	if joined != `{"city":"NYC"}` {
		t.Errorf("tool input reconstruction: got %q", joined)
	}

	// message_delta carries the mapped stop_reason and output tokens.
	md := rec.dataFor("message_delta")[0]
	if md["delta"].(map[string]any)["stop_reason"] != "tool_use" {
		t.Errorf("message_delta stop_reason: %+v", md["delta"])
	}
	// Cache-token usage must be surfaced on message_delta (JSON numbers decode to float64).
	usage := md["usage"].(map[string]any)
	if usage["cache_creation_input_tokens"] != float64(70) || usage["cache_read_input_tokens"] != float64(80) {
		t.Errorf("message_delta cache usage: got creation=%v read=%v, want 70/80",
			usage["cache_creation_input_tokens"], usage["cache_read_input_tokens"])
	}
}

// --- prompt-cache TTL ---

// bedrockBridgeHarness is bedrockHarness with a chosen default model, for tests
// whose behaviour depends on the Bedrock model ID.
func bedrockBridgeHarness(t *testing.T, rec *bedrockRecorder, model string) *failoverHarness {
	h := newFailoverHarness(t, rec.serve, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.BEDROCK
		primary.Failover = models.LLMFailover{}
		primary.DefaultModel = model
		primary.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "secret", "aws_region": "us-east-1"}
	})
	bedrockVendor.InvalidateClientCache(h.primary.ID)
	t.Cleanup(func() { bedrockVendor.InvalidateClientCache(h.primary.ID) })
	return h
}

// A cache_control ttl on the /anthropic/ bridge reaches Bedrock on the wire;
// a marker without one is sent exactly as before.
func TestAnthropicBridge_CacheTTLOnTheWire(t *testing.T) {
	const model = "anthropic.claude-sonnet-4-5-20250929-v1:0"
	body := func(cc string) string {
		return `{"model":"claude-sonnet-4-5","max_tokens":64,` +
			`"system":[{"type":"text","text":"sys","cache_control":` + cc + `}],` +
			`"tools":[{"name":"f","description":"d","input_schema":{"type":"object"},"cache_control":` + cc + `}],` +
			`"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":` + cc + `}]}]}`
	}

	t.Run("one hour", func(t *testing.T) {
		rec := &bedrockRecorder{status: http.StatusOK, reply: []byte(converseAnswer)}
		h := bedrockBridgeHarness(t, rec, model)
		resp, out := h.post("/anthropic/primary/v1/messages", body(`{"type":"ephemeral","ttl":"1h"}`))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

		rec.mu.Lock()
		sent := string(rec.body)
		rec.mu.Unlock()
		assert.Equal(t, 3, strings.Count(sent, `"cachePoint":{"ttl":"1h","type":"default"}`), sent)
	})

	t.Run("no ttl", func(t *testing.T) {
		rec := &bedrockRecorder{status: http.StatusOK, reply: []byte(converseAnswer)}
		h := bedrockBridgeHarness(t, rec, model)
		resp, out := h.post("/anthropic/primary/v1/messages", body(`{"type":"ephemeral"}`))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

		rec.mu.Lock()
		sent := string(rec.body)
		rec.mu.Unlock()
		assert.Equal(t, 3, strings.Count(sent, `"cachePoint":{"type":"default"}`), sent)
		assert.NotContains(t, sent, `"ttl"`)
	})
}

const oneHourModel = "anthropic.claude-sonnet-4-5-20250929-v1:0"

// cacheTTLRequest has a cache marker with control cc on a system block, a
// message block and a tool.
func cacheTTLRequest(cc *AnthropicCacheControl) *AnthropicMessagesRequest {
	ccJSON, _ := json.Marshal(cc)
	return &AnthropicMessagesRequest{
		System: json.RawMessage(`[{"type":"text","text":"sys","cache_control":` + string(ccJSON) + `}]`),
		Messages: []AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"hi","cache_control":` + string(ccJSON) + `}]`)},
		},
		Tools: []AnthropicTool{
			{Name: "f", Description: "d", InputSchema: map[string]any{"type": "object"}, CacheControl: cc},
		},
	}
}

// cachePoints returns the system, message and tool cache points of a Converse
// input, failing the test when one is missing.
func cachePoints(t *testing.T, modelID string, req *AnthropicMessagesRequest) (sys, msg, tool types.CachePointBlock) {
	t.Helper()
	input, err := (&Proxy{}).buildConverseInputFromAnthropic(modelID, req)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(input.System) != 2 {
		t.Fatalf("expected system text + cachepoint, got %d", len(input.System))
	}
	s, ok := input.System[1].(*types.SystemContentBlockMemberCachePoint)
	if !ok {
		t.Fatalf("expected system cachepoint, got %T", input.System[1])
	}
	if len(input.Messages) != 1 || len(input.Messages[0].Content) != 2 {
		t.Fatalf("expected text + cachepoint, got %+v", input.Messages)
	}
	m, ok := input.Messages[0].Content[1].(*types.ContentBlockMemberCachePoint)
	if !ok {
		t.Fatalf("expected message cachepoint, got %T", input.Messages[0].Content[1])
	}
	if input.ToolConfig == nil || len(input.ToolConfig.Tools) != 2 {
		t.Fatalf("expected toolspec + cachepoint, got %+v", input.ToolConfig)
	}
	tp, ok := input.ToolConfig.Tools[1].(*types.ToolMemberCachePoint)
	if !ok {
		t.Fatalf("expected tool cachepoint, got %T", input.ToolConfig.Tools[1])
	}
	return s.Value, m.Value, tp.Value
}

func TestAnthropicCacheTTL_SystemOneHour(t *testing.T) {
	sys, _, _ := cachePoints(t, oneHourModel, cacheTTLRequest(&AnthropicCacheControl{Type: "ephemeral", TTL: "1h"}))
	if sys.Type != types.CachePointTypeDefault || sys.Ttl != types.CacheTTLOneHour {
		t.Errorf("system cache point = %+v, want default type with 1h ttl", sys)
	}
}

func TestAnthropicCacheTTL_MessageAndToolOneHour(t *testing.T) {
	_, msg, tool := cachePoints(t, oneHourModel, cacheTTLRequest(&AnthropicCacheControl{Type: "ephemeral", TTL: "1h"}))
	if msg.Ttl != types.CacheTTLOneHour {
		t.Errorf("message cache point ttl = %q, want 1h", msg.Ttl)
	}
	if tool.Ttl != types.CacheTTLOneHour {
		t.Errorf("tool cache point ttl = %q, want 1h", tool.Ttl)
	}
}

func TestAnthropicCacheTTL_DefaultCases(t *testing.T) {
	for _, ttl := range []string{"", "5m", "2h"} {
		t.Run("ttl="+ttl, func(t *testing.T) {
			sys, msg, tool := cachePoints(t, oneHourModel, cacheTTLRequest(&AnthropicCacheControl{Type: "ephemeral", TTL: ttl}))
			for name, cp := range map[string]types.CachePointBlock{"system": sys, "message": msg, "tool": tool} {
				if cp.Type != types.CachePointTypeDefault || cp.Ttl != "" {
					t.Errorf("%s cache point = %+v, want default type and no ttl", name, cp)
				}
			}
		})
	}
}

// The ttl is decoded from the wire body.
func TestAnthropicCacheTTL_FromWireJSON(t *testing.T) {
	body := `{"model":"claude-sonnet-4-5","max_tokens":64,
		"system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral","ttl":"1h"}}],
		"tools":[{"name":"f","description":"d","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral","ttl":"1h"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`
	var req AnthropicMessagesRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	sys, msg, tool := cachePoints(t, oneHourModel, &req)
	for name, cp := range map[string]types.CachePointBlock{"system": sys, "message": msg, "tool": tool} {
		if cp.Ttl != types.CacheTTLOneHour {
			t.Errorf("%s cache point ttl = %q, want 1h", name, cp.Ttl)
		}
	}
}

// Claude 3 models only have 5-minute caching, and Bedrock rejects an extended
// ttl on non-Anthropic models: the cache points stay, the ttl goes.
func TestAnthropicCacheTTL_UnsupportedModelsDropTTL(t *testing.T) {
	for _, model := range []string{
		"anthropic.claude-3-7-sonnet-20250219-v1:0",
		"us.anthropic.claude-3-7-sonnet-20250219-v1:0",
		"us.anthropic.claude-3-5-sonnet-20241022-v2:0",
		"amazon.nova-lite-v1:0",
		"apac.amazon.nova-pro-v1:0",
		"global.amazon.nova-2-lite-v1:0",
	} {
		t.Run(model, func(t *testing.T) {
			sys, msg, tool := cachePoints(t, model, cacheTTLRequest(&AnthropicCacheControl{Type: "ephemeral", TTL: "1h"}))
			for name, cp := range map[string]types.CachePointBlock{"system": sys, "message": msg, "tool": tool} {
				if cp.Type != types.CachePointTypeDefault || cp.Ttl != "" {
					t.Errorf("%s cache point = %+v, want default type and no ttl", name, cp)
				}
			}
		})
	}
}
