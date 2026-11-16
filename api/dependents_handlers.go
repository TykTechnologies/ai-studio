package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
)

// DependentsResponse is the JSON:API-style envelope for a dependents lookup.
// Every array in attributes is always present (empty when nothing references
// the object). Dependents the caller may not see are left out of the arrays
// and counted in hidden; total counts them all.
// @Description Dependents response model
type DependentsResponse struct {
	Data struct {
		Type       string              `json:"type"`
		ID         string              `json:"id"`
		Attributes services.Dependents `json:"attributes"`
	} `json:"data"`
}

// dependentsLookup is one object type's "does it exist" check paired with
// its dependents query, so the six handlers share one body.
type dependentsLookup struct {
	exists func(id uint) (bool, error)
	fetch  func(id uint) (*services.Dependents, error)
	// catalogues is the kind of the Catalogues list (LLM catalogues when
	// nil): tool dependents list tool catalogues, datasource dependents
	// data catalogues.
	catalogues *catalogueKind
}

// hideDependents drops, from each list of a Workspace-scoped kind, the
// dependents the caller may not see, and counts them in Hidden (Total is
// unchanged). Kinds that are read-global (LLMs, tools, datasources,
// embedders: D4) and chats are left as they are.
func (a *API) hideDependents(c *gin.Context, d *services.Dependents, catalogues catalogueKind) bool {
	if !a.workspaceChecksOn(c) {
		return true
	}
	lists := []struct {
		refs     *[]services.DependentRef
		resource string
		model    interface{}
	}{
		{&d.Apps, "apps", &models.App{}},
		{&d.Catalogues, catalogues.resource, catalogues.model},
		{&d.Agents, "agents", &models.AgentConfig{}},
		{&d.ModelRouters, "model-routers", &models.ModelRouter{}},
		{&d.SemanticRouters, "semantic-routers", &models.SemanticRouter{}},
	}
	for _, l := range lists {
		if len(*l.refs) == 0 {
			continue
		}
		ids := make([]uint, len(*l.refs))
		for i, r := range *l.refs {
			ids[i] = r.ID
		}
		ws, err := a.workspaceOf(l.model, ids)
		if err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
			return false
		}
		kept := make([]services.DependentRef, 0, len(*l.refs))
		for _, r := range *l.refs {
			if a.visibleIn(c, l.resource, ws[r.ID]) {
				kept = append(kept, r)
			} else {
				d.Hidden++
			}
		}
		if c.IsAborted() {
			return false
		}
		*l.refs = kept
	}
	return true
}

func (a *API) rowExists(model interface{}, id uint) (bool, error) {
	err := a.service.DB.Select("id").First(model, id).Error
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func (a *API) respondDependents(c *gin.Context, lookup dependentsLookup) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "Invalid ID format"}}})
		return
	}

	found, err := lookup.exists(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Not Found", Detail: "Object not found"}}})
		return
	}

	deps, err := lookup.fetch(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Internal Server Error", Detail: err.Error()}}})
		return
	}

	kind := llmCatalogueKind
	if lookup.catalogues != nil {
		kind = *lookup.catalogues
	}
	if !a.hideDependents(c, deps, kind) {
		return
	}

	var response DependentsResponse
	response.Data.Type = "dependents"
	response.Data.ID = strconv.FormatUint(id, 10)
	response.Data.Attributes = *deps
	c.JSON(http.StatusOK, response)
}

// @Summary Get LLM dependents
// @Description List the apps, catalogues, agents, failover primaries and model routers that reference an LLM
// @Tags llms
// @Produce json
// @Param id path int true "LLM ID"
// @Success 200 {object} DependentsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /llms/{id}/dependents [get]
// @Security BearerAuth
func (a *API) getLLMDependents(c *gin.Context) {
	a.respondDependents(c, dependentsLookup{
		exists: func(id uint) (bool, error) { return a.rowExists(&models.LLM{}, id) },
		fetch:  a.service.GetLLMDependents,
	})
}

// @Summary Get tool dependents
// @Description List the apps, tool catalogues, dependent tools, agents and chats that reference a tool
// @Tags tools
// @Produce json
// @Param id path int true "Tool ID"
// @Success 200 {object} DependentsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /tools/{id}/dependents [get]
// @Security BearerAuth
func (a *API) getToolDependents(c *gin.Context) {
	a.respondDependents(c, dependentsLookup{
		exists:     func(id uint) (bool, error) { return a.rowExists(&models.Tool{}, id) },
		fetch:      a.service.GetToolDependents,
		catalogues: &toolCatalogueKind,
	})
}

// @Summary Get datasource dependents
// @Description List the apps, data catalogues, agents and chats that reference a datasource
// @Tags datasources
// @Produce json
// @Param id path int true "Datasource ID"
// @Success 200 {object} DependentsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /datasources/{id}/dependents [get]
// @Security BearerAuth
func (a *API) getDatasourceDependents(c *gin.Context) {
	a.respondDependents(c, dependentsLookup{
		exists:     func(id uint) (bool, error) { return a.rowExists(&models.Datasource{}, id) },
		fetch:      a.service.GetDatasourceDependents,
		catalogues: &dataCatalogueKind,
	})
}

// @Summary Get filter dependents
// @Description List the LLMs, tools and chats a filter is attached to
// @Tags filters
// @Produce json
// @Param id path int true "Filter ID"
// @Success 200 {object} DependentsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /filters/{id}/dependents [get]
// @Security BearerAuth
func (a *API) getFilterDependents(c *gin.Context) {
	a.respondDependents(c, dependentsLookup{
		exists: func(id uint) (bool, error) { return a.rowExists(&models.Filter{}, id) },
		fetch:  a.service.GetFilterDependents,
	})
}

// @Summary Get app dependents
// @Description List the agents that reference an app (agent_configs.app_id); the other arrays are always present and empty
// @Tags apps
// @Produce json
// @Param id path int true "App ID"
// @Success 200 {object} DependentsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /apps/{id}/dependents [get]
// @Security BearerAuth
func (a *API) getAppDependents(c *gin.Context) {
	a.respondDependents(c, dependentsLookup{
		exists: func(id uint) (bool, error) { return a.rowExists(&models.App{}, id) },
		fetch:  a.service.GetAppDependents,
	})
}

// @Summary Get model router dependents
// @Description List what references a model router (nothing in the data model does today, so the arrays are empty)
// @Tags model-routers
// @Produce json
// @Param id path int true "Model Router ID"
// @Success 200 {object} DependentsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /model-routers/{id}/dependents [get]
// @Security BearerAuth
func (a *API) getModelRouterDependents(c *gin.Context) {
	a.respondDependents(c, dependentsLookup{
		exists: func(id uint) (bool, error) { return a.rowExists(&models.ModelRouter{}, id) },
		fetch:  a.service.GetModelRouterDependents,
	})
}

// @Summary Get secret dependents
// @Description List the LLMs, tools and datasources that read a secret through a $SECRET/name reference
// @Tags secrets
// @Produce json
// @Param id path int true "Secret ID"
// @Success 200 {object} DependentsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "When TYK_AI_SECRET_KEY is not set"
// @Router /secrets/{id}/dependents [get]
// @Security BearerAuth
func (a *API) getSecretDependents(c *gin.Context) {
	if !checkSecretKey(c) {
		return
	}

	// Only the name is needed, so read the row directly rather than through
	// GetSecretByID, which would decrypt a value this endpoint never returns.
	var secret secrets.Secret
	a.respondDependents(c, dependentsLookup{
		exists: func(id uint) (bool, error) {
			err := a.service.DB.Select("id", "var_name").First(&secret, id).Error
			if err == nil {
				return true, nil
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		},
		fetch: func(uint) (*services.Dependents, error) {
			return a.service.GetSecretDependents(secret.VarName)
		},
	})
}
