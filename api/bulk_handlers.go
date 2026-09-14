package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services/model_router"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// Bulk actions: POST /{type}/bulk {"action": "activate"|"deactivate"|"delete", "ids": [...]}.
//
// Each id goes through the same service call the single-object route uses,
// so soft delete, object hooks, failover validation, catalogue cleanup and
// audit all happen exactly as for one object, and one failure never stops
// the rest: the response reports every id with ok or an error string.
//
// The permission is the one the matching single route carries (publish for
// activate/deactivate, delete for delete); it is resolved from the body by
// bulkActionPermission before the handler runs.

// maxBulkIDs bounds one request; the UI selects from one page at a time.
const maxBulkIDs = 100

const (
	bulkActivate   = "activate"
	bulkDeactivate = "deactivate"
	bulkDelete     = "delete"
)

// BulkID is an object id that may arrive as a JSON number or as a numeric
// string: JSON:API rows carry string ids, so a list page posts back what it
// was given. Anything else fails to bind and the request is a 400.
type BulkID uint

func (id *BulkID) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	n, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
	if err != nil {
		return fmt.Errorf("ids must be positive integers or numeric strings, got %s", string(data))
	}
	*id = BulkID(n)
	return nil
}

// BulkActionInput is the request body of a bulk action.
// @Description Bulk action request
type BulkActionInput struct {
	Action string   `json:"action" example:"deactivate"`
	IDs    []BulkID `json:"ids" swaggertype:"array,integer"`
}

// BulkActionResult is one id's outcome.
// @Description One object's outcome in a bulk action
type BulkActionResult struct {
	ID    uint   `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// BulkActionResponse is the envelope of a bulk action.
// @Description Bulk action response
type BulkActionResponse struct {
	Data struct {
		Action    string             `json:"action"`
		Results   []BulkActionResult `json:"results"`
		Succeeded int                `json:"succeeded"`
		Failed    int                `json:"failed"`
	} `json:"data"`
}

// bulkTarget is one object type's bulk behaviour. setActive is nil for
// types without a live switch (filters, secrets), which makes activate and
// deactivate a 400 for them. before runs once, ahead of the loop, and may
// end the request (the secrets store's key check, the router edition gate).
type bulkTarget struct {
	what      string // singular, for messages: "LLM", "tool"
	resource  string // authz resource: "llms"
	before    func(c *gin.Context) bool
	setActive func(c *gin.Context, id uint, active bool) error
	remove    func(c *gin.Context, id uint) error
}

// bulkActionPermission resolves the route's permission from the body:
// publish for activate/deactivate, delete otherwise (so a malformed action
// needs the stricter grant and the handler then answers 400). The body is
// bound with ShouldBindBodyWith so the handler can read it again.
func bulkActionPermission(resource string) func(*gin.Context) authz.Permission {
	return func(c *gin.Context) authz.Permission {
		var input BulkActionInput
		if err := c.ShouldBindBodyWith(&input, binding.JSON); err == nil {
			switch input.Action {
			case bulkActivate, bulkDeactivate:
				return authz.Publish(resource)
			}
		}
		return authz.Delete(resource)
	}
}

func bulkBadRequest(c *gin.Context, detail string) {
	c.JSON(http.StatusBadRequest, ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: detail}},
	})
}

// bulkErrorMessage turns a service error into the per-id message: a missing
// row reads "LLM not found", everything else is the error text (the failover
// conflict, a plugin hook rejection, ...).
func bulkErrorMessage(err error, what string) string {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return what + " not found"
	}
	return err.Error()
}

// runBulkAction is the shared handler body.
func (a *API) runBulkAction(c *gin.Context, target bulkTarget) {
	if target.before != nil && !target.before(c) {
		return
	}

	var input BulkActionInput
	if err := c.ShouldBindBodyWith(&input, binding.JSON); err != nil {
		bulkBadRequest(c, err.Error())
		return
	}
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))

	switch input.Action {
	case bulkDelete:
	case bulkActivate, bulkDeactivate:
		if target.setActive == nil {
			bulkBadRequest(c, target.what+"s have no active flag; only delete is supported")
			return
		}
	default:
		bulkBadRequest(c, "unsupported action: "+input.Action+" (expected activate, deactivate or delete)")
		return
	}
	if len(input.IDs) == 0 {
		bulkBadRequest(c, "ids must contain at least one id")
		return
	}
	if len(input.IDs) > maxBulkIDs {
		bulkBadRequest(c, "ids must contain at most 100 ids")
		return
	}

	// The per-object permission checks that a single route performs in its
	// handler (rather than its annotation) apply once here: activating needs
	// publish exactly as the dedicated route does.
	if input.Action != bulkDelete && !a.requirePublish(c, target.resource) {
		return
	}

	var response BulkActionResponse
	response.Data.Action = input.Action
	response.Data.Results = make([]BulkActionResult, 0, len(input.IDs))
	for _, bulkID := range input.IDs {
		id := uint(bulkID)
		var err error
		switch input.Action {
		case bulkDelete:
			err = target.remove(c, id)
		default:
			err = target.setActive(c, id, input.Action == bulkActivate)
		}
		// The edition gate is a property of the request, not of one id.
		if errors.Is(err, model_router.ErrEnterpriseFeature) {
			c.JSON(http.StatusPaymentRequired, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Error", Detail: err.Error()}},
			})
			return
		}
		result := BulkActionResult{ID: id, OK: err == nil}
		if err != nil {
			result.Error = bulkErrorMessage(err, target.what)
			response.Data.Failed++
		} else {
			response.Data.Succeeded++
		}
		response.Data.Results = append(response.Data.Results, result)
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Bulk activate, deactivate or delete LLMs
// @Description Apply one action to up to 100 LLMs. Each id is processed independently through the same path as the single-object route; the response lists every id with ok or an error. Requires llms:publish for activate/deactivate and llms:delete for delete.
// @Tags llms
// @Accept json
// @Produce json
// @Param input body BulkActionInput true "Action and ids"
// @Success 200 {object} BulkActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /llms/bulk [post]
// @Security BearerAuth
func (a *API) bulkLLMs(c *gin.Context) {
	a.runBulkAction(c, bulkTarget{
		what:     "LLM",
		resource: "llms",
		setActive: func(c *gin.Context, id uint, active bool) error {
			// Going live needs every "required to publish" metadata field,
			// as on the single route.
			if active {
				if err := a.publishGateError(c.Request.Context(), models.GovernedObjectTypeLLM, models.BuiltinObjectID(id)); err != nil {
					return err
				}
			}
			_, err := a.service.SetLLMActive(id, active, currentUserID(c))
			if err == nil && a.proxy != nil {
				a.proxy.Reload()
			}
			return err
		},
		remove: func(c *gin.Context, id uint) error {
			if err := a.service.DeleteLLM(id); err != nil {
				return err
			}
			a.removeGovernedMetadata(c, models.GovernedObjectTypeLLM, models.BuiltinObjectID(id))
			return nil
		},
	})
}

// @Summary Bulk activate, deactivate or delete tools
// @Description Apply one action to up to 100 tools. Each id is processed independently; the response lists every id with ok or an error. Requires tools:publish for activate/deactivate and tools:delete for delete.
// @Tags tools
// @Accept json
// @Produce json
// @Param input body BulkActionInput true "Action and ids"
// @Success 200 {object} BulkActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /tools/bulk [post]
// @Security BearerAuth
func (a *API) bulkTools(c *gin.Context) {
	a.runBulkAction(c, bulkTarget{
		what:     "tool",
		resource: "tools",
		setActive: func(c *gin.Context, id uint, active bool) error {
			if active {
				if err := a.publishGateError(c.Request.Context(), models.GovernedObjectTypeTool, models.BuiltinObjectID(id)); err != nil {
					return err
				}
			}
			_, err := a.service.SetToolActive(id, active, currentUserID(c))
			return err
		},
		remove: func(c *gin.Context, id uint) error {
			if err := a.service.DeleteTool(id); err != nil {
				return err
			}
			a.removeGovernedMetadata(c, models.GovernedObjectTypeTool, models.BuiltinObjectID(id))
			return nil
		},
	})
}

// @Summary Bulk activate, deactivate or delete datasources
// @Description Apply one action to up to 100 datasources. Each id is processed independently; the response lists every id with ok or an error. Requires datasources:publish for activate/deactivate and datasources:delete for delete.
// @Tags datasources
// @Accept json
// @Produce json
// @Param input body BulkActionInput true "Action and ids"
// @Success 200 {object} BulkActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /datasources/bulk [post]
// @Security BearerAuth
func (a *API) bulkDatasources(c *gin.Context) {
	a.runBulkAction(c, bulkTarget{
		what:     "datasource",
		resource: "datasources",
		setActive: func(c *gin.Context, id uint, active bool) error {
			if active {
				if err := a.publishGateError(c.Request.Context(), models.GovernedObjectTypeDatasource, models.BuiltinObjectID(id)); err != nil {
					return err
				}
			}
			_, err := a.service.SetDatasourceActive(id, active, currentUserID(c))
			return err
		},
		remove: func(c *gin.Context, id uint) error {
			if err := a.service.DeleteDatasource(id); err != nil {
				return err
			}
			a.removeGovernedMetadata(c, models.GovernedObjectTypeDatasource, models.BuiltinObjectID(id))
			return nil
		},
	})
}

// @Summary Bulk activate, deactivate or delete apps
// @Description Apply one action to up to 100 apps. Each id is processed independently; the response lists every id with ok or an error. Requires apps:publish for activate/deactivate and apps:delete for delete.
// @Tags apps
// @Accept json
// @Produce json
// @Param input body BulkActionInput true "Action and ids"
// @Success 200 {object} BulkActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /apps/bulk [post]
// @Security BearerAuth
func (a *API) bulkApps(c *gin.Context) {
	a.runBulkAction(c, bulkTarget{
		what:     "app",
		resource: "apps",
		setActive: func(c *gin.Context, id uint, active bool) error {
			_, err := a.service.SetAppActive(id, active, currentUserID(c))
			return err
		},
		remove: func(c *gin.Context, id uint) error { return a.service.DeleteApp(id) },
	})
}

// @Summary Bulk delete filters
// @Description Delete up to 100 filters. Filters have no active flag, so activate and deactivate are rejected. Each id is processed independently; the response lists every id with ok or an error. Requires filters:delete.
// @Tags filters
// @Accept json
// @Produce json
// @Param input body BulkActionInput true "Action and ids"
// @Success 200 {object} BulkActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /filters/bulk [post]
// @Security BearerAuth
func (a *API) bulkFilters(c *gin.Context) {
	a.runBulkAction(c, bulkTarget{
		what:     "filter",
		resource: "filters",
		remove:   func(c *gin.Context, id uint) error { return a.service.DeleteFilter(id) },
	})
}

// @Summary Bulk delete secrets
// @Description Delete up to 100 secrets. Secrets have no active flag, so activate and deactivate are rejected. Each id is processed independently; the response lists every id with ok or an error. Requires secrets:delete.
// @Tags secrets
// @Accept json
// @Produce json
// @Param input body BulkActionInput true "Action and ids"
// @Success 200 {object} BulkActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "When TYK_AI_SECRET_KEY is not set"
// @Router /secrets/bulk [post]
// @Security BearerAuth
func (a *API) bulkSecrets(c *gin.Context) {
	a.runBulkAction(c, bulkTarget{
		what:     "secret",
		resource: "secrets",
		before:   checkSecretKey,
		remove: func(c *gin.Context, id uint) error {
			// DeleteSecretByID is silent about a missing row; the single
			// route accepts that, but a bulk result should say which ids
			// did nothing.
			found, err := a.rowExists(&secrets.Secret{}, id)
			if err != nil {
				return err
			}
			if !found {
				return gorm.ErrRecordNotFound
			}
			return secrets.DeleteSecretByID(a.config.DB, id)
		},
	})
}

// @Summary Bulk activate, deactivate or delete model routers
// @Description Apply one action to up to 100 model routers (Enterprise only). Each id is processed independently; the response lists every id with ok or an error. Requires model-routers:publish for activate/deactivate and model-routers:delete for delete.
// @Tags model-routers
// @Accept json
// @Produce json
// @Param input body BulkActionInput true "Action and ids"
// @Success 200 {object} BulkActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 403 {object} ErrorResponse
// @Router /model-routers/bulk [post]
// @Security BearerAuth
func (a *API) bulkModelRouters(c *gin.Context) {
	a.runBulkAction(c, bulkTarget{
		what:     "model router",
		resource: "model-routers",
		setActive: func(c *gin.Context, id uint, active bool) error {
			return a.service.ModelRouterService.ToggleRouterActive(id, active)
		},
		remove: func(c *gin.Context, id uint) error {
			if err := a.service.ModelRouterService.DeleteRouter(id); err != nil {
				return err
			}
			if a.service.SystemEvents != nil {
				a.service.SystemEvents.EmitModelRouterDeleted(id, 0)
			}
			return nil
		},
	})
}
