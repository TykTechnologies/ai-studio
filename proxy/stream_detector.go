package proxy

import (
	"context"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// streamDetectionMiddleware inspects incoming requests to detect streaming intent
// and stores the decision in the request context for downstream routing
func (p *Proxy) streamDetectionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		llmSlug := vars["llmSlug"]

		// DEBUG: Log incoming request details
		log.Debug().
			Str("path", r.URL.Path).
			Str("llmSlug", llmSlug).
			Int64("content_length", r.ContentLength).
			Msg("streamDetectionMiddleware entry")

		// Get LLM configuration from proxy cache
		llm, ok := p.GetLLM(llmSlug)
		if !ok {
			respondWithError(w, http.StatusNotFound, "LLM not found", nil, false)
			return
		}

		// Detect streaming intent based on vendor and request
		isStreaming, err := switches.DetectStreamingIntent(llm.Vendor, r)
		if err != nil {
			log.Error().Err(err).Str("vendor", string(llm.Vendor)).Msg("Failed to detect streaming intent")
			respondWithError(w, http.StatusBadRequest, "Failed to detect streaming intent", err, false)
			return
		}

		log.Debug().Bool("isStreaming", isStreaming).Str("vendor", string(llm.Vendor)).Msg("streamDetectionMiddleware detected")

		// Store streaming decision in request context
		ctx := context.WithValue(r.Context(), "is_streaming_request", isStreaming)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
