//go:build enterprise
// +build enterprise

package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/gin-gonic/gin"
)

// NamespaceResponse represents a namespace in API responses
type NamespaceResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Name       string `json:"name"`
		EdgeCount  int64  `json:"edge_count"`
		LLMCount   int64  `json:"llm_count"`
		AppCount   int64  `json:"app_count"`
		TokenCount int64  `json:"token_count"`
	} `json:"attributes"`
}

// NamespaceListResponse represents a list of namespaces
type NamespaceListResponse struct {
	Data []NamespaceResponse `json:"data"`
}

// ReloadResponse represents a reload operation response
type ReloadResponse struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		OperationID     string `json:"operation_id"`
		TargetNamespace string `json:"target_namespace"`
		Status          string `json:"status"`
		Message         string `json:"message"`
	} `json:"attributes"`
}

// @Summary List namespaces
// @Description Get a list of available namespaces with statistics
// @Tags namespaces
// @Accept json
// @Produce json
// @Success 200 {object} NamespaceListResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/namespaces [get]
// @Security BearerAuth
func (a *API) listNamespaces(c *gin.Context) {
	namespaces, err := a.service.NamespaceService.ListNamespaces()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	response := NamespaceListResponse{
		Data: make([]NamespaceResponse, len(namespaces)),
	}

	for i, ns := range namespaces {
		response.Data[i] = NamespaceResponse{
			Type: "namespaces",
			ID:   ns.Name,
			Attributes: struct {
				Name       string `json:"name"`
				EdgeCount  int64  `json:"edge_count"`
				LLMCount   int64  `json:"llm_count"`
				AppCount   int64  `json:"app_count"`
				TokenCount int64  `json:"token_count"`
			}{
				Name:       ns.Name,
				EdgeCount:  ns.EdgeCount,
				LLMCount:   ns.LLMCount,
				AppCount:   ns.AppCount,
				TokenCount: ns.TokenCount,
			},
		}
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Trigger namespace reload
// @Description Push the current configuration to every edge in a namespace (connected, registered, or offline for less than 5 minutes; offline edges are waited for until the push's deadline). Returns the push operation (ID "push-..."); follow it with GET /api/v1/reload-operations/{operation_id}/status. 409 when the namespace has no edge to push to (up to v2.2: 404).
// @Tags namespaces
// @Accept json
// @Produce json
// @Param namespace path string true "Namespace name (use 'global' for global namespace)"
// @Success 202 {object} ReloadResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/namespaces/{namespace}/reload [post]
// @Security BearerAuth
func (a *API) triggerNamespaceReload(c *gin.Context) {
	namespace := c.Param("namespace")

	// Security: Validate namespace parameter
	if err := validateNamespace(namespace); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
		return
	}

	res, err := a.service.NamespaceService.TriggerNamespaceReload(namespace, initiatedBy(c))
	if err != nil {
		sendPushError(c, err)
		return
	}
	pushAccepted(c, res)
}

// @Summary Get edges in namespace
// @Description Get all edge instances in a specific namespace
// @Tags namespaces
// @Accept json
// @Produce json
// @Param namespace path string true "Namespace name (use 'global' for global namespace)"
// @Success 200 {object} EdgeListResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/namespaces/{namespace}/edges [get]
// @Security BearerAuth
func (a *API) getNamespaceEdges(c *gin.Context) {
	namespace := c.Param("namespace")

	// Security: Validate namespace parameter
	if err := validateNamespace(namespace); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	edges, err := a.service.NamespaceService.GetEdgesInNamespace(namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// Serialize response
	response := EdgeListResponse{
		Data: make([]EdgeResponse, len(edges)),
		Meta: struct {
			TotalCount int64 `json:"total_count"`
			TotalPages int   `json:"total_pages"`
			PageSize   int   `json:"page_size"`
			PageNumber int   `json:"page_number"`
		}{
			TotalCount: int64(len(edges)),
			TotalPages: 1,
			PageSize:   len(edges),
			PageNumber: 1,
		},
	}

	for i, edge := range edges {
		response.Data[i] = serializeEdgeWithHealth(&edge)
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Get reload operation status
// @Description Get the status of a push ("reload") operation, per edge (Enterprise; 402 in Community Edition). status is in_progress until every edge has answered or the deadline passed, then one of succeeded, succeeded_with_warnings, partially_failed, failed or expired (up to v2.2: completed, failed or timed_out).
// @Tags namespaces
// @Accept json
// @Produce json
// @Param operation_id path string true "Operation ID"
// @Success 200 {object} ReloadResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/reload-operations/{operation_id}/status [get]
// @Security BearerAuth
func (a *API) getReloadOperationStatus(c *gin.Context) {
	a.pushStatus(c)
}
