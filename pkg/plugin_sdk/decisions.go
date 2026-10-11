package plugin_sdk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
)

// Decision-model types, in the OpenAI Decisions shape. A decision request
// reaches a plugin in its vendor's wire format: OpenAI's /v1/decisions or the
// System One /v1/systemone format (TypeSafe Jev and wire-compatible
// self-hosts). ParseDecisionRequest and ParseDecisionResponse read either, so
// a plugin handles one schema whatever the upstream.
type (
	DecisionRequest   = decisions.Request
	DecisionQuestion  = decisions.Question
	DecisionChoice    = decisions.Choice
	DecisionLevel     = decisions.Level
	DecisionResponse  = decisions.Response
	DecisionAnswer    = decisions.Answer
	DecisionUsage     = decisions.Usage
	DecisionSummary   = decisions.Summary
	DecisionAnswerPct = decisions.AnswerProbability
)

// Decision question and answer types.
const (
	DecisionPredicate = decisions.TypePredicate
	DecisionChoiceQ   = decisions.TypeChoice
	DecisionScore     = decisions.TypeScore
	DecisionRefusal   = decisions.TypeRefusal
)

// IsDecisionPath reports whether a request path targets a decision endpoint
// (/v1/decisions or /v1/systemone). In hooks that see the request path it
// agrees with Context.IsDecision.
func IsDecisionPath(path string) bool {
	return decisions.IsDecisionPath(path)
}

// ParseDecisionRequest reads a decision request body sent to path, in either
// wire format, as an OpenAI-shape request. System One questions keep their
// order, and choice options theirs.
func ParseDecisionRequest(path string, body []byte) (*DecisionRequest, error) {
	format, ok := decisions.FormatForPath(path)
	if !ok {
		format = decisions.FormatOpenAI
	}
	return decisions.ParseRequest(body, format)
}

// ParseDecisionResponse reads a decision response body in either wire format
// as an OpenAI-shape response. req, when given (from ParseDecisionRequest),
// puts System One answers in question order and marks questions the model
// left unanswered as refusals.
func ParseDecisionResponse(body []byte, req *DecisionRequest) (*DecisionResponse, error) {
	return decisions.ParseResponse(body, req)
}

// CallDecision asks the decision model behind llmID (Studio agent plugins)
// and returns its answers in the OpenAI shape, whatever the LLM's vendor. The
// call goes through the gateway with the agent App's credential, so access,
// filters, plugins, budgets and analytics apply as for any App. An empty
// req.Model uses the LLM's default model. Needs the llms.proxy scope.
func CallDecision(ctx context.Context, llmID uint32, req *DecisionRequest) (*DecisionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	out, err := ai_studio_sdk.CallDecision(ctx, llmID, body)
	if err != nil {
		return nil, err
	}
	var resp DecisionResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("unreadable decision response: %w", err)
	}
	return &resp, nil
}

// DecisionAnswerText renders a decision response one answer per line
// ("topic: choice=billing (confidence 0.9)"), the text response filters see.
func DecisionAnswerText(resp *DecisionResponse) string {
	return decisions.RenderAnswers(resp)
}
