package proxy

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/TykTechnologies/midsommar/v2/models"
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
)

// vertexGeminiPathMiddleware maps a Gemini API path on /llm/call/{slug} to the
// Vertex path of a Vertex LLM (vertexVendor.NativePathFromGeminiAPI). The
// /ai/ and /v1 bridges drive Vertex LLMs with the Google AI client, which
// sends Gemini API paths; Vertex's wire format is the same, only the path
// differs. It runs first on the route, so model validation, stream detection
// and the handlers all see the one Vertex path. It is path translation only,
// so it needs no trust in the internal hop: an external /llm/call caller
// using a Gemini API path gets the same mapping.
func (p *Proxy) vertexGeminiPathMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := mux.Vars(r)["llmSlug"]
		llm, ok := p.GetLLM(slug)
		if !ok || llm.Vendor != models.VERTEX {
			next.ServeHTTP(w, r)
			return
		}
		prefix := fmt.Sprintf("/llm/call/%s", slug)
		native, changed, err := vertexVendor.NativePathFromGeminiAPI(llm, strings.TrimPrefix(r.URL.Path, prefix))
		if err != nil {
			respondWithError(w, http.StatusBadRequest, err.Error(), nil, false)
			return
		}
		if changed {
			r = r.Clone(r.Context())
			r.URL.Path = prefix + native
			r.URL.RawPath = ""
		}
		next.ServeHTTP(w, r)
	})
}
