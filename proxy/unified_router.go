package proxy

// Unified router endpoint: a fixed OpenRouter-style ingress (/v1/chat/completions,
// /v1/completions) that accepts OpenAI-format requests with "vendor/model" model
// strings. The vendor prefix is resolved against the gateway's LLM route table and
// the request is rewritten to the existing /ai/{routeId}/v1/... shim path, then
// handed to the /ai/ handler chain directly — no HTTP hop and no new middleware.
// Auth, app→LLM access checks, AllowedModels enforcement, and streaming all run
// exactly as they do for a direct /ai/{slug}/... call because the rewrite happens
// before that chain.
//
// Note the deliberate difference from the per-vendor endpoints: /ai/{slug}/...
// treats the model field as a bare vendor model name, so this router strips the
// prefix before forwarding.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ParseUnifiedModel splits an OpenRouter-style model string into its vendor route
// prefix and the bare model name. The split is on the FIRST slash so model names
// that themselves contain slashes (e.g. fine-tune identifiers) survive intact.
func ParseUnifiedModel(model string) (route string, bareModel string, err error) {
	idx := strings.Index(model, "/")
	if idx <= 0 || idx == len(model)-1 {
		return "", "", fmt.Errorf("model '%s' is not in '<vendor>/<model>' format", model)
	}
	return model[:idx], model[idx+1:], nil
}

// NewUnifiedRouterHandler returns the handler for the unified router ingress.
// hasRoute reports whether an LLM route with the given slug exists; next receives
// the rewritten request and is expected to be the /ai/ handler chain (auth
// middleware included).
func NewUnifiedRouterHandler(hasRoute func(slug string) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			respondWithOAIError(w, http.StatusMethodNotAllowed, "only POST is supported on the unified router endpoint", nil, false)
			return
		}

		var bodyBytes []byte
		if r.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(r.Body)
			r.Body.Close()
			if err != nil {
				respondWithOAIError(w, http.StatusBadRequest, "failed to read request body", err, false)
				return
			}
		}

		// Decode into RawMessage so every field except "model" is forwarded verbatim
		// (no float64 round-trip corrupting numbers the client sent).
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(bodyBytes, &fields); err != nil {
			respondWithOAIError(w, http.StatusBadRequest, "invalid request body: expected an OpenAI-format JSON object", err, false)
			return
		}

		var model string
		if raw, ok := fields["model"]; ok {
			if err := json.Unmarshal(raw, &model); err != nil {
				respondWithOAIError(w, http.StatusBadRequest, "invalid request body: 'model' must be a string", err, false)
				return
			}
		}

		route, bareModel, err := ParseUnifiedModel(model)
		if err != nil {
			respondWithOAIError(w, http.StatusBadRequest,
				fmt.Sprintf("model '%s' must be in '<vendor>/<model>' format (e.g. 'openai/gpt-4o'); the vendor prefix is the LLM route slug configured on this gateway", model),
				nil, false)
			return
		}

		if !hasRoute(route) {
			respondWithOAIError(w, http.StatusNotFound,
				fmt.Sprintf("vendor '%s' not found or not supported by your access rights", route),
				nil, false)
			return
		}

		// The per-vendor shim endpoints expect a bare model name, so strip the prefix.
		modelJSON, err := json.Marshal(bareModel)
		if err != nil {
			respondWithOAIError(w, http.StatusInternalServerError, "failed to rewrite model field", err, false)
			return
		}
		fields["model"] = modelJSON

		newBody, err := json.Marshal(fields)
		if err != nil {
			respondWithOAIError(w, http.StatusInternalServerError, "failed to rewrite request body", err, false)
			return
		}

		// /v1/<rest> -> /ai/<route>/v1/<rest>; the /ai/ mux extracts routeId itself.
		r.URL.Path = "/ai/" + route + r.URL.Path
		r.Body = io.NopCloser(bytes.NewReader(newBody))
		r.ContentLength = int64(len(newBody))

		next.ServeHTTP(w, r)
	})
}
