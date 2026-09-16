package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
)

// The admin App handlers and the portal's createUserApp bind Tyk-managed
// MCP servers the same way: validate before the App is written (so a
// refusal leaves nothing behind), then replace the bindings once it exists.
// These helpers hold that sequence so the three call sites agree on the
// error mapping and on the reload.

// adminAppActor identifies the caller of an admin App handler for the
// visibility check: an administrator, or anyone who may write Apps, sees
// every server; other callers are limited to their teams' catalogues.
func adminAppActor(c *gin.Context) (uint, bool) {
	actorUser, _ := c.Get("user")
	if u, ok := actorUser.(*models.User); ok && u != nil {
		return u.ID, u.IsAdmin || authz.Can(c, authz.Write("apps"))
	}
	return 0, true
}

// validateAppMCPBindings checks that every requested server may be bound
// by this actor next to these LLMs. It writes the error response itself and
// reports whether the caller may go on.
func (a *API) validateAppMCPBindings(c *gin.Context, actorID uint, actorAdmin bool, llmIDs, serverIDs []uint) bool {
	if _, err := a.service.ValidateMCPServerBindings(actorID, actorAdmin, llmIDs, serverIDs); err != nil {
		if !mcpBindingError(c, err) {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		}
		return false
	}
	return true
}

// applyAppMCPServers replaces the App's bindings and returns the App
// reloaded with them. It writes the error response itself.
func (a *API) applyAppMCPServers(c *gin.Context, app *models.App, serverIDs []uint) (*models.App, bool) {
	reloaded, err := a.service.SetAppMCPServers(app.ID, serverIDs)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return app, false
	}
	if reloaded != nil {
		return reloaded, true
	}
	return app, true
}
