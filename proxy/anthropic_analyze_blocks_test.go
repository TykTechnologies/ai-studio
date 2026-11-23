package proxy

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// A non-streamed Anthropic response used to be dropped from accounting (no
// chat record, no proxy log, no budget cost) as soon as it held a content
// block other than text or tool_use, and the raw block was written to the log
// (midsommar-clean-6bg5.5). Every block type is accounted now, with the token
// counts the vendor reported, and no block content reaches a log.
func TestAnalyzeResponse_AnthropicAccountsEveryContentBlockType(t *testing.T) {
	const (
		model = "claude-luna"
		// Markers that would show a raw block in a log line.
		signature = "EsYDCsQCsignaturemarker"
		secret    = "model-output-marker"
		toolUse   = `{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"Oslo"}}`
	)

	for _, tc := range []struct {
		name    string
		content string
		tools   int
	}{
		{"empty thinking block", `[{"type":"thinking","thinking":"","signature":"` + signature + `"},` + toolUse + `]`, 1},
		{"non-empty thinking block", `[{"type":"thinking","thinking":"` + secret + `","signature":"` + signature + `"},` + toolUse + `]`, 1},
		{"redacted thinking", `[{"type":"redacted_thinking","data":"` + signature + `"},{"type":"text","text":"hi"}]`, 0},
		{"server tool use", `[{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"` + secret + `"}},
			{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[{"type":"web_search_result","title":"` + secret + `"}]},
			{"type":"text","text":"hi"}]`, 0},
		{"unknown block type", `[{"type":"future_block","payload":"` + secret + `"},` + toolUse + `]`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &chatRecordCounter{}
			analytics.SetHandler(h)
			defer analytics.ResetHandler()

			// Capture both loggers at their most verbose level.
			var logged bytes.Buffer
			log.SetOutput(&logged)
			defer log.SetOutput(io.Discard)
			prevOut, prevLevel := logrus.StandardLogger().Out, logrus.GetLevel()
			logrus.SetOutput(&logged)
			logrus.SetLevel(logrus.DebugLevel)
			defer func() {
				logrus.SetOutput(prevOut)
				logrus.SetLevel(prevLevel)
			}()

			svc := new(MockService)
			svc.On("GetModelPriceByModelNameAndVendor", mock.Anything, string(models.ANTHROPIC)).
				Return(&models.ModelPrice{ModelName: model, Vendor: string(models.ANTHROPIC), CPT: 0.00001, CPIT: 0.00001, Currency: "USD"}, nil)

			body := []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"` + model + `","content":` + tc.content + `,
				"stop_reason":"end_turn","usage":{"input_tokens":21,"output_tokens":13}}`)
			req, err := http.NewRequest(http.MethodPost, "http://gw/llm/rest/claude/v1/messages", nil)
			require.NoError(t, err)

			AnalyzeResponse(svc, &models.LLM{ID: 1, Vendor: models.ANTHROPIC, DontLogBodies: true}, &models.App{ID: 2}, http.StatusOK, body, nil, req)

			assert.Equal(t, 1, h.logCount(), "the call writes its proxy log")
			if assert.Equal(t, 1, h.count(), "the call writes its chat record") {
				rec := h.records[0]
				assert.Equal(t, model, rec.Name)
				assert.Equal(t, 21, rec.PromptTokens)
				assert.Equal(t, 13, rec.ResponseTokens)
				assert.Equal(t, tc.tools, rec.ToolCalls)
				assert.Greater(t, rec.Cost, 0.0, "the call is charged")
			}

			out := logged.String()
			assert.NotContains(t, out, "failed to analyze response")
			assert.NotContains(t, out, signature, "a block's signature or data must not be logged")
			assert.NotContains(t, out, secret, "a block's content must not be logged")
		})
	}
}
