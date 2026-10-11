// Package systemoneVendor serves decision models over the System One wire
// format (POST /v1/systemone): TypeSafe's hosted Jev API and wire-compatible
// self-hosts such as llama.cpp serving Kev, OpenJev or Laya. It is a
// decisions-only vendor: there is no chat driver and no embedder.
package systemoneVendor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/embeddings"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/schema"
)

// DefaultAPIEndpoint is TypeSafe's hosted System One API, used when an LLM
// has no API endpoint of its own.
const DefaultAPIEndpoint = "https://api.typesafe.ai"

// ErrDecisionsOnly is returned where a generative capability is asked of
// this vendor.
var ErrDecisionsOnly = errors.New("systemone LLMs serve decision models only; call them through /v1/decisions or /llm/rest/{slug}/v1/systemone")

type SystemOne struct{}

func New() models.LLMVendorProvider {
	return &SystemOne{}
}

func (v *SystemOne) GetTokenCounts(choice *llms.ContentChoice) (int, int, int) {
	return 0, 0, 0
}

func (v *SystemOne) GetDriver(LLMConfig *models.LLM, settings *models.LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error) {
	return nil, ErrDecisionsOnly
}

func (v *SystemOne) GetEmbedder(spec *models.EmbedderSpec) (*embeddings.EmbedderImpl, error) {
	return nil, ErrDecisionsOnly
}

func (v *SystemOne) AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	if !decisions.IsDecisionPath(r.URL.Path) {
		return nil, nil, nil, fmt.Errorf("[analyse response] unknown systemone endpoint: %s", r.URL.Path)
	}
	dr, err := responses.NewDecisionResponse(body)
	if err != nil {
		return nil, nil, nil, err
	}
	return llm, app, dr, nil
}

func (v *SystemOne) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	return nil, nil, nil, fmt.Errorf("systemone decision models do not stream")
}

func (v *SystemOne) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	if llm.APIKey != "" {
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))
	}
	return nil
}

// ProxyScreenRequest rejects streaming and bodies that are not System One
// requests. Question semantics are left to the upstream, which answers 422
// with the offending field.
func (v *SystemOne) ProxyScreenRequest(llm *models.LLM, r *http.Request, isStreamingChannel bool) error {
	if isStreamingChannel {
		return fmt.Errorf("streaming is not supported for decision requests")
	}
	b, err := helpers.CopyRequestBody(r)
	if err != nil {
		return err
	}
	var req struct {
		Stream    bool            `json:"stream"`
		State     json.RawMessage `json:"state"`
		Questions json.RawMessage `json:"questions"`
	}
	if err := json.Unmarshal(b, &req); err != nil {
		return err
	}
	if req.Stream {
		return fmt.Errorf("streaming is not supported for decision requests")
	}
	if !decisions.IsDecisionPath(r.URL.Path) {
		return fmt.Errorf("systemone LLMs only serve %s", decisions.SystemOneEndpoint)
	}
	if len(bytes.TrimSpace(req.State)) == 0 {
		return fmt.Errorf("'state' is required")
	}
	if q := bytes.TrimSpace(req.Questions); len(q) == 0 || q[0] != '{' {
		return fmt.Errorf("'questions' must be an object of named questions")
	}
	return nil
}

func (v *SystemOne) ProvidesEmbedder() bool {
	return false
}

func (v *SystemOne) ProvidesChat() bool {
	return false
}

func (v *SystemOne) DecisionEndpoint() string {
	return decisions.SystemOneEndpoint
}
