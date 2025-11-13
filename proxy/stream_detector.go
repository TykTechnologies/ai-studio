package proxy

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/gorilla/mux"
)

// streamDetectionMiddleware inspects incoming requests to detect streaming intent
// and stores the decision in the request context for downstream routing
func (p *Proxy) streamDetectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		llmSlug := vars["llmSlug"]

		// Get LLM configuration from proxy cache
		llm, ok := p.GetLLM(llmSlug)
		if !ok {
			respondWithError(w, http.StatusNotFound, "LLM not found", nil, false)
			return
		}

		// Detect streaming intent based on vendor and request
		isStreaming, err := switches.DetectStreamingIntent(llm.Vendor, r)
		if err != nil {
			slog.Error("Failed to detect streaming intent", "error", err, "vendor", llm.Vendor)
			respondWithError(w, http.StatusBadRequest, "Failed to detect streaming intent", err, false)
			return
		}

		// Store streaming decision in request context
		ctx := context.WithValue(r.Context(), "is_streaming_request", isStreaming)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
