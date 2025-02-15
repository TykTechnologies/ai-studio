package proxy

import (
	"context"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/services"
)

type CredentialExtractor func(r *http.Request) (string, error)

type CredentialValidator struct {
	service    services.ServiceInterface
	p          *Proxy
	validators map[string]CredentialExtractor
}

func NewCredentialValidator(service services.ServiceInterface, proxy *Proxy) *CredentialValidator {
	return &CredentialValidator{
		service:    service,
		p:          proxy,
		validators: make(map[string]CredentialExtractor),
	}
}

func (cv *CredentialValidator) RegisterValidator(vendor string, validator CredentialExtractor) {
	cv.validators[strings.ToLower(vendor)] = validator
}

func (cv *CredentialValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse the URL path to extract the slug
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 3 {
			respondWithError(w, http.StatusBadRequest, "invalid request path", nil)
			return
		}

		var llmSlug, dsSlug, routeID string
		switch pathParts[1] {
		case "llm":
			if len(pathParts) > 3 {
				llmSlug = pathParts[3] // For /llm/stream/{llmSlug}
			} else {
				llmSlug = pathParts[2] // For /llm/{llmSlug}
			}
		case "datasource":
			dsSlug = pathParts[2]
		case "ai":
			routeID = pathParts[2]
		default:
			respondWithError(w, http.StatusBadRequest, "invalid request path, options are llm or datasource", nil)
			return
		}

		if llmSlug == "" && dsSlug == "" && routeID == "" {
			respondWithError(w, http.StatusBadRequest, "no LLM, datasource, or interface specified", nil)
			return
		}

		var token string
		var err error

		if dsSlug != "" {
			token = r.Header.Get("Authorization")
			if token == "" {
				respondWithError(w, http.StatusUnauthorized, "missing authorization header", nil)
				return
			}
			// Strip Bearer prefix if present
			token = strings.TrimPrefix(token, "Bearer ")
		} else if llmSlug != "" {
			llm, ok := cv.p.GetLLM(llmSlug)
			if !ok {
				respondWithError(w, http.StatusNotFound, "[cred validator] LLM not found "+llmSlug, nil)
				return
			}
			extractor, ok := cv.validators[strings.ToLower(string(llm.Vendor))]
			if !ok {
				respondWithError(w, http.StatusBadRequest, "no validator for this vendor", nil)
				return
			}
			token, err = extractor(r)
			if err != nil {
				respondWithError(w, http.StatusUnauthorized, "invalid credential for llm pass through", err)
				return
			}
		} else if routeID != "" {
			hVal := r.Header.Get("Authorization")
			parts := strings.Split(hVal, "Bearer ")
			if len(parts) != 2 {
				respondWithError(w, http.StatusUnauthorized, "missing or malformed authorization header", nil)
			}

			token = parts[1]
			if token == "" {
				respondWithError(w, http.StatusUnauthorized, "missing or malformed authorization header", nil)
				return
			}
		}

		var ok bool
		ok, r = cv.CheckCredential(token, dsSlug, llmSlug, routeID, r)

		if !ok {
			respondWithError(w, http.StatusUnauthorized, "invalid credential", nil)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (cv *CredentialValidator) CheckCredential(token, dsSlug, llmSlug, routeID string, r *http.Request) (bool, *http.Request) {
	cred, err := cv.service.GetCredentialBySecret(token)
	if err != nil || !cred.Active {
		return false, r
	}

	app, err := cv.service.GetAppByCredentialID(cred.ID)
	if err != nil {
		return false, r
	}

	ctx := context.WithValue(r.Context(), "app", app)
	r = r.WithContext(ctx)

	if dsSlug != "" {
		ds, ok := cv.p.GetDatasource(dsSlug)
		if !ok {
			return false, r
		}
		for _, d := range app.Datasources {
			if d.ID == ds.ID {
				return true, r
			}
		}

		return false, r
	}

	if llmSlug != "" {
		llm, ok := cv.p.GetLLM(llmSlug)
		if !ok {
			return false, r
		}

		for _, l := range app.LLMs {
			if l.ID == llm.ID {
				return true, r
			}
		}

		return false, r
	}

	if routeID != "" {
		px, ok := cv.p.GetLLM(routeID)
		if !ok {
			return false, r
		}

		for _, llm := range app.LLMs {
			if llm.ID == px.ID {
				return true, r
			}
		}

		return false, r
	}

	return false, r
}

// func (cv *CredentialValidator) Middleware(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		vars := mux.Vars(r)
// 		llmSlug := vars["llmSlug"]
// 		dsSlug := vars["dsSlug"]

// 		if llmSlug == "" && dsSlug == "" {
// 			respondWithError(w, http.StatusBadRequest, "no LLM or datasource specified", nil)
// 			return
// 		}

// 		// it's a DS query
// 		if dsSlug != "" {
// 			ds, ok := cv.p.GetDatasource(dsSlug)

// 			if !ok {
// 				respondWithError(w, http.StatusNotFound, "datasource not found", nil)
// 				return
// 			}

// 			token := r.Header.Get("Authorization")
// 			if token == "" {
// 				respondWithError(w, http.StatusUnauthorized, "missing authorization header", nil)
// 				return
// 			}

// 			allow := cv.CheckCredential(token, ds, nil)
// 			if !allow {
// 				respondWithError(w, http.StatusUnauthorized, "invalid credential", nil)
// 				return
// 			}
// 		}

// 		// it's an LLM query
// 		if llmSlug != "" {
// 			llm, ok := cv.p.GetLLM(llmSlug)

// 			if !ok {
// 				respondWithError(w, http.StatusNotFound, "LLM not found", nil)
// 				return
// 			}

// 			extractor, ok := cv.validators[string(llm.Vendor)]
// 			if !ok {
// 				respondWithError(w, http.StatusUnauthorized, "no validator for this vendor", nil)
// 			}

// 			token, err := extractor(r)
// 			if err != nil {
// 				respondWithError(w, http.StatusUnauthorized, "invalid credential", nil)
// 			}

// 			allow := cv.CheckCredential(token, nil, llm)
// 			if !allow {
// 				respondWithError(w, http.StatusUnauthorized, "invalid credential", nil)
// 				return
// 			}

// 		}

// 		next.ServeHTTP(w, r)
// 	})
// }

// func (cv *CredentialValidator) CheckCredential(token string, ds *models.Datasource, llm *models.LLM) bool {
// 	cred, err := cv.service.GetCredentialBySecret(token)
// 	if err != nil {
// 		return false
// 	}

// 	if !cred.Active {
// 		return false
// 	}

// 	app, err := cv.service.GetAppByCredentialID(cred.ID)
// 	if err != nil {
// 		return false
// 	}

// 	if ds != nil {
// 		for _, d := range app.Datasources {
// 			if d.ID == ds.ID {
// 				return true
// 			}
// 		}

// 		return false
// 	}

// 	if llm != nil {
// 		for _, l := range app.LLMs {
// 			if l.ID == llm.ID {
// 				return true
// 			}
// 		}

// 		return false
// 	}

// 	return false
// }
