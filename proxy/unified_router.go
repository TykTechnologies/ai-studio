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
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// maxUnifiedRouterBodyBytes caps how much of a /v1/ completion request body the
// router will buffer for model extraction. Generous enough for vision payloads,
// small enough that oversized bodies cannot exhaust gateway memory.
const maxUnifiedRouterBodyBytes = 10 << 20 // 10 MB

// unifiedRouteSlugPattern is the only shape a route prefix may take. The prefix
// becomes a URL path segment, so this is defense-in-depth against traversal or
// control characters even though unknown slugs cannot match a real route.
var unifiedRouteSlugPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

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
// next receives the rewritten request and is expected to be the /ai/ handler
// chain (auth middleware included).
//
// The handler deliberately does NOT resolve the route prefix itself: every
// well-formed request is rewritten and forwarded so that authentication runs
// before route resolution. An anonymous caller therefore always gets 401 and
// cannot enumerate configured route slugs from 404-vs-401 responses; the /ai/
// shim produces the vendor-not-found 404 after auth.
func NewUnifiedRouterHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only the completion endpoints carry a routable model field. Everything
		// else under /v1/ (e.g. GET /v1/models) passes through untouched so the
		// authenticated chain can serve or reject it.
		if r.Method != http.MethodPost ||
			(r.URL.Path != "/v1/chat/completions" && r.URL.Path != "/v1/completions") {
			next.ServeHTTP(w, r)
			return
		}

		var bodyBytes []byte
		if r.Body != nil {
			var err error
			bodyBytes, err = io.ReadAll(http.MaxBytesReader(w, r.Body, maxUnifiedRouterBodyBytes))
			r.Body.Close()
			if err != nil {
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) {
					respondWithOAIError(w, http.StatusRequestEntityTooLarge,
						fmt.Sprintf("request body exceeds the %d byte limit", maxUnifiedRouterBodyBytes), nil, false)
					return
				}
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
		if err != nil || !unifiedRouteSlugPattern.MatchString(route) {
			respondWithOAIError(w, http.StatusBadRequest,
				fmt.Sprintf("model '%s' must be in '<vendor>/<model>' format (e.g. 'openai/gpt-4o'); the vendor prefix is the LLM route slug configured on this gateway", model),
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

// unifiedModelInfo is one entry of the OpenAI-compatible model list.
type unifiedModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

type unifiedModelList struct {
	Object string             `json:"object"`
	Data   []unifiedModelInfo `json:"data"`
}

// handleUnifiedListModels serves GET /v1/models. It is registered BEHIND the
// credential middleware and lists only the routes the authenticated app is
// associated with, as "{routeSlug}/{model}" ids ready to send back to the
// unified router. Model names come from each route's AllowedModels plus its
// DefaultModel; a route with neither is omitted (its model names are unknown).
func (p *Proxy) handleUnifiedListModels(w http.ResponseWriter, r *http.Request) {
	app, ok := r.Context().Value("app").(*models.App)
	if !ok || app == nil {
		respondWithOAIError(w, http.StatusUnauthorized, "authentication required", nil, true)
		return
	}

	accessible := make(map[uint]bool, len(app.LLMs))
	for _, l := range app.LLMs {
		accessible[l.ID] = true
	}

	entries := make([]unifiedModelInfo, 0)
	p.mu.RLock()
	for routeSlug, conf := range p.llms {
		if !accessible[conf.ID] {
			continue
		}
		seen := make(map[string]bool, len(conf.AllowedModels)+1)
		for _, m := range append(append([]string{}, conf.AllowedModels...), conf.DefaultModel) {
			if m == "" || seen[m] {
				continue
			}
			seen[m] = true
			entries = append(entries, unifiedModelInfo{
				ID:      routeSlug + "/" + m,
				Object:  "model",
				OwnedBy: string(conf.Vendor),
			})
		}
	}
	p.mu.RUnlock()

	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(unifiedModelList{Object: "list", Data: entries})
}
