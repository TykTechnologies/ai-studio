package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/gorilla/mux"
)

// handleAnthropicListModels serves GET /anthropic/{routeId}/v1/models, which Claude Code
// calls at startup when CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY is set, so its /model
// picker can show the Bedrock model this connection really serves.
//
// Auth runs in the credential-validator middleware, as for /v1/messages. The connection
// checks mirror handleAnthropicMessagesEntry and the model choice mirrors
// resolveAnthropicModelID; keep them in step, or the list can offer a model /v1/messages
// refuses. A missing or refused default model lists nothing rather than erroring, so
// Claude Code falls back to its built-in list. No model is called, so nothing is billed,
// filtered or recorded. The limit query parameter is ignored.
func (p *Proxy) handleAnthropicListModels(w http.ResponseWriter, r *http.Request) {
	routeID := mux.Vars(r)["routeId"]

	// GetLLM takes the read lock; Reload writes this map concurrently.
	conf, ok := p.GetLLM(routeID)
	if !ok {
		respondWithAnthropicError(w, http.StatusNotFound, "not_found_error", "route not found")
		return
	}

	if conf.Vendor != models.BEDROCK {
		respondWithAnthropicError(w, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("the Anthropic Messages bridge only supports Bedrock LLMs, not %q", conf.Vendor))
		return
	}

	modelID := bedrockVendor.GetModelID(conf, "")
	if modelID == "" || !NewModelValidator(conf.AllowedModels).IsModelAllowed(modelID) {
		writeAnthropicModelList(w, nil)
		return
	}

	// The id goes out unchanged: Claude Code (v2.1.223+) keeps any id containing
	// "claude" or "anthropic", which region-prefixed inference profile ids satisfy.
	displayName := modelID
	if conf.Name != "" {
		displayName = conf.Name + " — " + modelID
	}
	writeAnthropicModelList(w, []AnthropicModelListEntry{{
		Type:        "model",
		ID:          modelID,
		DisplayName: displayName,
	}})
}

// writeAnthropicModelList writes a 200 model list, encoding an empty list as [] not null.
func writeAnthropicModelList(w http.ResponseWriter, entries []AnthropicModelListEntry) {
	if entries == nil {
		entries = []AnthropicModelListEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(AnthropicModelListResponse{Data: entries, HasMore: false})
}
