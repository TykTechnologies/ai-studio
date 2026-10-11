package responses

import (
	"encoding/json"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
)

// DecisionResponse is the analysed response of a decision-model call, in
// either decision wire format. Decision models are billed by input tokens and
// usually report no output tokens; each answer counts as one choice.
type DecisionResponse struct {
	GenericResponse
	Questions int
	Refusals  int
	// answered is set when the body carried answers. An error body (an
	// upstream failure, a budget denial, a policy block) has none, and its
	// questions were not refused by the model.
	answered bool
}

// NewDecisionResponse analyses a decision response body in either format. A
// JSON body without answers (an upstream error, a budget denial, a policy
// block) analyses to zero answers so the exchange is still logged, like a
// failed chat call; a missing usage block yields zero tokens. Only a body
// that is not JSON at all is an error.
func NewDecisionResponse(body []byte) (*DecisionResponse, error) {
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse decision response: %w", err)
	}
	out := &DecisionResponse{}
	out.Model = probe.Model
	if u, ok := decisions.UsageFromBody(body); ok {
		out.PromptTokens = u.InputTokens
		out.CompletionTokens = u.OutputTokens
		// OpenAI's input_tokens includes the cached share, as its chat
		// prompt_tokens does; the chat record subtracts and prices it the
		// same way for both.
		if d := u.InputTokensDetails; d != nil {
			out.CacheReadPromptTokens = d.CachedTokens
			out.CacheWritePromptTokens = d.CacheWriteTokens
		}
	}
	resp, err := decisions.ParseResponse(body, nil)
	if err != nil {
		return out, nil
	}
	out.answered = true
	for _, a := range resp.Answers {
		if a.Type == decisions.TypeRefusal {
			out.Refusals++
			continue
		}
		out.Choices++
	}
	out.Questions = out.Choices + out.Refusals
	return out, nil
}

// ApplyRequest recounts questions and refusals against the request body that
// produced the response. System One responses omit unanswered questions, so
// only the request knows how many were asked.
func (d *DecisionResponse) ApplyRequest(reqBody []byte, format decisions.Format) {
	req, err := decisions.ParseRequest(reqBody, format)
	if err != nil || len(req.Questions) == 0 {
		return
	}
	d.Questions = len(req.Questions)
	if d.answered && d.Choices < d.Questions {
		d.Refusals = d.Questions - d.Choices
	}
}

func (d *DecisionResponse) GetDecisionQuestions() int {
	return d.Questions
}

func (d *DecisionResponse) GetDecisionRefusals() int {
	return d.Refusals
}
