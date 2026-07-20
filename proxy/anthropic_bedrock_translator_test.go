package proxy

import (
	"encoding/json"
	"testing"

	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
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
		types.StopReasonToolUse, 11, 22,
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
			Usage: &types.TokenUsage{InputTokens: aws.Int32(10), OutputTokens: aws.Int32(5)},
		}},
	}
	for _, e := range events {
		if blocked := st.handleEvent(e); blocked {
			t.Fatal("unexpected filter block")
		}
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
}
