package api

import (
	"errors"
	"io"
	"net/http"

	appconfig "github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/agenttracing"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// maxDevImportBytes bounds one imported document (a 5,000-span session is
// a few MB).
const maxDevImportBytes = 64 << 20

// registerAgentTracingDevRoutes adds the e2e seed import (jyx.35) in
// DEVMODE only: POST /agent-tracing/dev/import stores one JSON v1 session
// document. It needs traces:write, and does not exist outside DEVMODE.
func (a *API) registerAgentTracingDevRoutes(g *permRouter, devMode bool) {
	if !devMode {
		return
	}
	log.Warn().Msg("DEVMODE: the agent tracing seed import (POST /api/v1/agent-tracing/dev/import) is enabled")
	g.POST("/dev/import", authz.Write("traces"), a.importAgentTraceDocument)
}

func devModeEnabled() bool { return appconfig.Get("").DevMode }

func (a *API) importAgentTraceDocument(c *gin.Context) {
	svc := a.agentTracing()
	imp, ok := svc.(agenttracing.DevImporter)
	if !ok {
		err := agenttracing.ErrDisabled
		if !svc.IsEnterpriseAvailable() {
			err = agenttracing.ErrEnterpriseFeature
		}
		respondAgentTracing(c, nil, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxDevImportBytes))
	if err != nil {
		agentTracingError(c, http.StatusBadRequest, "Bad Request", "the document could not be read")
		return
	}
	id, err := imp.ImportDocument(c.Request.Context(), body)
	if err != nil {
		var bad *agenttracing.BadRequestError
		if errors.As(err, &bad) {
			agentTracingError(c, http.StatusBadRequest, "Bad Request", bad.Msg)
			return
		}
		respondAgentTracing(c, nil, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"session_id": id})
}
