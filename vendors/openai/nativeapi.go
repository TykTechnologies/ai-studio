package openaiVendor

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
)

// ResourceResponse is the native resource type of a Responses API response.
const ResourceResponse = "openai.response"

// responseRetention is how long OpenAI keeps a stored response.
const responseRetention = 30 * 24 * time.Hour

// responseIDCharset is the shape of an OpenAI response ID (resp_67cc...).
var responseIDCharset = regexp.MustCompile(`^resp_[A-Za-z0-9_-]{1,200}$`)

// nativeHeaderAllow is every header of the client request that reaches OpenAI
// on a native operation. OpenAI-Organization and OpenAI-Project are left out:
// responses live in the LLM's vendor project, whichever the caller names.
var nativeHeaderAllow = []string{"content-type", "accept", "openai-beta"}

// ResponsesFilterFormat is the filter format of a Responses API body: the
// proxy reads instructions, input items and prompt variables for the request
// filters (and refuses a body it cannot read).
const ResponsesFilterFormat = "openai_responses"

// InputFilterFormat is the filter format of an embeddings, completions or
// moderations body: the proxy reads "input", "prompt" and "suffix" (and
// refuses token arrays, which it cannot screen, when filters are attached).
const InputFilterFormat = "openai_input"

// accountHeaders pick another organization or project than the LLM's API key
// defaults to. They are dropped from every Responses request, so a response is
// created, read and polled in the same project.
var accountHeaders = []string{"openai-organization", "openai-project"}

// responseIDs are the response IDs inside a response object.
var responseIDs = []nativeapi.BodyRef{
	{Path: "id", Resource: ResourceResponse},
	{Path: "previous_response_id", Resource: ResourceResponse},
	conversationRef,
}

// previousResponseRef is the context a request continues from: a previous
// response, or a conversation.
var previousResponseRef = []nativeapi.BodyRef{{Path: "previous_response_id", Resource: ResourceResponse}, conversationRef}

var nativeSpec = nativeapi.Spec{
	Resources: append([]nativeapi.ResourceType{responseResource, conversationResource}, batchResources...),
	Operations: append(append(append([]nativeapi.Operation{}, responseOperations...), conversationOperations...),
		batchOperations...),
	ChainHooks: responseChainHooks,
}

var responseResource = nativeapi.ResourceType{
	Name:      ResourceResponse,
	IDCharset: responseIDCharset,
	// Only background responses are ledgered: their usage is not known
	// when the create returns. Every other response is accounted by the
	// synchronous chain (AnalyzeResponse), which skips background ones.
	Async: &nativeapi.AsyncSpec{
		Poll: func(vendorID string) nativeapi.UpstreamReq {
			return nativeapi.UpstreamReq{Method: http.MethodGet, Path: "/v1/responses/" + vendorID}
		},
		State:       responseState,
		ObjectUsage: responseObjectUsage,
		Cancel: func(vendorID string) nativeapi.UpstreamReq {
			return nativeapi.UpstreamReq{Method: http.MethodPost, Path: "/v1/responses/" + vendorID + "/cancel"}
		},
		Tracked: isBackgroundResponse,
	},
}

var responseOperations = []nativeapi.Operation{
	{
		Name:         "openai.responses.retrieve",
		Method:       http.MethodGet,
		Path:         "/v1/responses/{id}",
		PathResource: ResourceResponse,
		Ledger:       nativeapi.LedgerRead,
		ResponseIDs:  responseIDs,
		// stream=true resumes a background response as server-sent
		// events, which the operation does not serve.
		Upstream: nativeapi.UpstreamSpec{
			HeaderAllow: nativeHeaderAllow,
			QueryAllow:  []string{"include", "include[]"},
			RefuseQuery: []string{"stream"},
		},
		ErrorShape: NativeError,
	},
	{
		Name:         "openai.responses.delete",
		Method:       http.MethodDelete,
		Path:         "/v1/responses/{id}",
		PathResource: ResourceResponse,
		Ledger:       nativeapi.LedgerDelete,
		ResponseIDs:  []nativeapi.BodyRef{{Path: "id", Resource: ResourceResponse}},
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:   NativeError,
	},
	{
		Name:         "openai.responses.cancel",
		Method:       http.MethodPost,
		Path:         "/v1/responses/{id}/cancel",
		PathResource: ResourceResponse,
		Ledger:       nativeapi.LedgerMutate,
		ResponseIDs:  responseIDs,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:   NativeError,
	},
	{
		// A page of input items: their IDs (msg_...) are not response IDs.
		Name:         "openai.responses.input_items",
		Method:       http.MethodGet,
		Path:         "/v1/responses/{id}/input_items",
		PathResource: ResourceResponse,
		Ledger:       nativeapi.LedgerNone,
		Upstream: nativeapi.UpstreamSpec{
			HeaderAllow: nativeHeaderAllow,
			QueryAllow:  []string{"after", "before", "limit", "order", "include", "include[]"},
		},
		ErrorShape: NativeError,
	},
}

var responseChainHooks = &nativeapi.ChainHooks{Hooks: []nativeapi.ChainHook{
	{
		Name:         "openai.responses.create",
		Method:       http.MethodPost,
		Path:         "/v1/responses",
		RequestRefs:  previousResponseRef,
		FilterFormat: ResponsesFilterFormat,
		ResponseIDs:  responseIDs,
		EventObject:  "response",
		DropHeaders:  accountHeaders,
		ErrorShape:   NativeError,
	},
	{
		Name:         "openai.responses.input_tokens",
		Method:       http.MethodPost,
		Path:         "/v1/responses/input_tokens",
		RequestRefs:  previousResponseRef,
		FilterFormat: ResponsesFilterFormat,
		DropHeaders:  accountHeaders,
		ErrorShape:   NativeError,
	},
	{
		Name:         "openai.responses.compact",
		Method:       http.MethodPost,
		Path:         "/v1/responses/compact",
		RequestRefs:  previousResponseRef,
		FilterFormat: ResponsesFilterFormat,
		DropHeaders:  accountHeaders,
		ErrorShape:   NativeError,
	},
	// Prompt-and-input calls: only the filter format, so the filters read
	// "input" / "prompt" instead of an absent "messages".
	{Name: "openai.embeddings", Method: http.MethodPost, Path: "/v1/embeddings", FilterFormat: InputFilterFormat, ErrorShape: NativeError},
	{Name: "openai.completions", Method: http.MethodPost, Path: "/v1/completions", FilterFormat: InputFilterFormat, ErrorShape: NativeError},
	{Name: "openai.moderations", Method: http.MethodPost, Path: "/v1/moderations", FilterFormat: InputFilterFormat, ErrorShape: NativeError},
}}

// NativeAPI implements nativeapi.Provider: ownership of Responses API
// responses on the native /llm routes.
func (v *OpenAI) NativeAPI() nativeapi.Spec { return nativeSpec }

// NativeError renders an OpenAI-shaped error body. The proxy names error
// types the way Anthropic does; OpenAI calls all client errors
// invalid_request_error.
func NativeError(status int, typ, msg string) []byte {
	switch typ {
	case "not_found_error", "permission_error", "authentication_error":
		typ = "invalid_request_error"
	case "api_error":
		typ = "server_error"
	}
	b, _ := json.Marshal(map[string]any{
		"error": map[string]any{"message": msg, "type": typ, "param": nil, "code": nil},
	})
	return b
}

// responseObject is the part of a Responses API response object the gateway
// reads.
type responseObject struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Status      string `json:"status"`
	Background  bool   `json:"background"`
	Model       string `json:"model"`
	ServiceTier string `json:"service_tier"`
	CreatedAt   int64  `json:"created_at"`
	CompletedAt *int64 `json:"completed_at"`
	Output      []struct {
		Type string `json:"type"`
	} `json:"output"`
	Usage *struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		InputTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

func parseResponseObject(obj []byte) (responseObject, error) {
	var r responseObject
	if err := json.Unmarshal(obj, &r); err != nil {
		return r, err
	}
	if r.Object != "response" || r.ID == "" {
		return r, errors.New("not a response object")
	}
	return r, nil
}

// isBackgroundResponse reports whether obj is a response created with
// background: true.
func isBackgroundResponse(obj []byte) bool {
	r, err := parseResponseObject(obj)
	return err == nil && r.Background
}

// responseState maps a response's status onto the coarse lifecycle the
// reconciler drives. Stored responses are kept for 30 days.
func responseState(obj []byte) (nativeapi.StateInfo, error) {
	r, err := parseResponseObject(obj)
	if err != nil {
		return nativeapi.StateInfo{}, err
	}
	var info nativeapi.StateInfo
	switch r.Status {
	case "queued", "in_progress":
		info.State = nativeapi.StateInProgress
	default: // completed, failed, cancelled, incomplete
		info.State = nativeapi.StateEnded
		if r.CompletedAt != nil && *r.CompletedAt > 0 {
			t := time.Unix(*r.CompletedAt, 0).UTC()
			info.EndedAt = &t
		}
	}
	if r.CreatedAt > 0 {
		exp := time.Unix(r.CreatedAt, 0).UTC().Add(responseRetention)
		info.ResultsExpireAt = &exp
	}
	return info, nil
}

// responseObjectUsage is the billable usage of an ended response; a response
// without usage (cancelled before it ran) is not billed.
func responseObjectUsage(obj []byte) (nativeapi.Usage, bool, error) {
	r, err := parseResponseObject(obj)
	if err != nil {
		return nativeapi.Usage{}, false, err
	}
	if r.Usage == nil {
		return nativeapi.Usage{}, false, nil
	}
	return nativeapi.Usage{
		Model:                r.Model,
		InputTokens:          r.Usage.InputTokens,
		OutputTokens:         r.Usage.OutputTokens,
		CacheReadInputTokens: r.Usage.InputTokensDetails.CachedTokens,
		Tier:                 normalizeServiceTier(r.ServiceTier),
		Billed:               true,
	}, true, nil
}

// ResponsesUsage is the usage of one Responses API call, as the synchronous
// chain records it. Like chat completions, InputTokens includes the cached
// tokens.
type ResponsesUsage struct {
	Model                     string
	InputTokens, OutputTokens int
	CachedTokens              int
	ToolCalls                 int
	// Tier is the service tier the response reports, normalised
	// (normalizeServiceTier): "" for list price.
	Tier string
}

var _ models.IPriceAdjustedResponse = (*ResponsesUsage)(nil)

// ServiceTier and PriceMultiplier: a tiered response is billed at list price
// unless the LLM overrides the tier (LLM.PriceMultipliers), as a background
// response is.
func (u *ResponsesUsage) ServiceTier() string      { return u.Tier }
func (u *ResponsesUsage) PriceMultiplier() float64 { return 1 }

func (u *ResponsesUsage) GetPromptTokens() int           { return u.InputTokens }
func (u *ResponsesUsage) GetResponseTokens() int         { return u.OutputTokens }
func (u *ResponsesUsage) GetChoiceCount() int            { return 1 }
func (u *ResponsesUsage) GetToolCount() int              { return u.ToolCalls }
func (u *ResponsesUsage) GetModel() string               { return u.Model }
func (u *ResponsesUsage) GetCacheWritePromptTokens() int { return 0 }
func (u *ResponsesUsage) GetCacheReadPromptTokens() int  { return u.CachedTokens }

// responsesPathKind tells the Responses API paths the synchronous chain
// forwards apart: "create" (POST /v1/responses), "compact", "input_tokens",
// or "" for any other path.
func responsesPathKind(path string) string {
	p := strings.TrimSuffix(strings.ToLower(path), "/")
	switch {
	case strings.HasSuffix(p, "/responses"):
		return "create"
	case strings.HasSuffix(p, "/responses/compact"):
		return "compact"
	case strings.HasSuffix(p, "/responses/input_tokens"):
		return "input_tokens"
	}
	return ""
}

// usageOf turns a response object into the usage the synchronous chain
// records. A background response is accounted by the native reconciler once
// it ends, so it carries no usage here (nil: log only), and neither does a
// response without usage.
func usageOf(r responseObject) *ResponsesUsage {
	if r.Background || r.Usage == nil {
		return nil
	}
	u := &ResponsesUsage{
		Model:        r.Model,
		InputTokens:  r.Usage.InputTokens,
		OutputTokens: r.Usage.OutputTokens,
		CachedTokens: r.Usage.InputTokensDetails.CachedTokens,
		Tier:         normalizeServiceTier(r.ServiceTier),
	}
	for _, o := range r.Output {
		if strings.HasSuffix(o.Type, "_call") {
			u.ToolCalls++
		}
	}
	return u
}

// analyzeResponsesBody is AnalyzeResponse for a Responses API path. Only a
// successful create or compact carries usage; input_tokens counts tokens and
// is logged without a chat record, like Anthropic's count_tokens.
func analyzeResponsesBody(kind string, statusCode int, body []byte) (models.ITokenResponse, error) {
	if statusCode < 200 || statusCode >= 300 || kind == "input_tokens" {
		return nil, nil
	}
	var r responseObject
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	if u := usageOf(r); u != nil {
		return u, nil
	}
	return nil, nil
}

// analyzeResponsesStream reads a streamed Responses API create: the usage is
// on the response object of the terminal event (response.completed,
// response.incomplete or response.failed).
func analyzeResponsesStream(statusCode int, stream []byte) (models.ITokenResponse, error) {
	if statusCode < 200 || statusCode >= 300 {
		return nil, nil
	}
	var last *ResponsesUsage
	background := false
	for _, line := range bytes.Split(stream, []byte("\n")) {
		data, ok := bytes.CutPrefix(bytes.TrimRight(line, "\r"), []byte("data:"))
		if !ok {
			continue
		}
		data = bytes.TrimSpace(data)
		if len(data) == 0 || data[0] != '{' {
			continue
		}
		var ev struct {
			Type     string          `json:"type"`
			Response *responseObject `json:"response"`
		}
		if json.Unmarshal(data, &ev) != nil || ev.Response == nil {
			continue
		}
		background = background || ev.Response.Background
		if u := usageOf(*ev.Response); u != nil {
			last = u
		}
	}
	if background || last == nil {
		return nil, nil
	}
	return last, nil
}
