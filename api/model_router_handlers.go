package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/model_router"
	"github.com/gin-gonic/gin"
)

// ModelRouterInput represents the JSON:API input for creating/updating a model router
type ModelRouterInput struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name        string              `json:"name"`
			Slug        string              `json:"slug"`
			Description string              `json:"description"`
			APICompat   string              `json:"api_compat"`
			Active      bool                `json:"active"`
			Namespace   string              `json:"namespace"`
			Pools       []ModelPoolInput    `json:"pools"`
		} `json:"attributes"`
	} `json:"data"`
}

// ModelPoolInput represents a pool in the input
type ModelPoolInput struct {
	Name               string            `json:"name"`
	ModelPattern       string            `json:"model_pattern"`
	SelectionAlgorithm string            `json:"selection_algorithm"`
	Priority           int               `json:"priority"`
	Vendors            []PoolVendorInput `json:"vendors"`
}

// PoolVendorInput represents a vendor in the input
type PoolVendorInput struct {
	LLMID    uint                `json:"llm_id"`
	Weight   int                 `json:"weight"`
	Active   bool                `json:"active"`
	Mappings []ModelMappingInput `json:"mappings"`
}

// ModelMappingInput represents a mapping in the input
type ModelMappingInput struct {
	SourceModel string `json:"source_model"`
	TargetModel string `json:"target_model"`
}

// @Summary Create a new model router
// @Description Create a new model router with pools, vendors, and mappings (Enterprise only)
// @Tags model-routers
// @Accept json
// @Produce json
// @Param router body ModelRouterInput true "Model router information"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 500 {object} ErrorResponse
// @Router /model-routers [post]
// @Security BearerAuth
func (a *API) createModelRouter(c *gin.Context) {
	var input ModelRouterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	router := a.inputToModelRouter(&input)

	if err := a.service.ModelRouterService.CreateRouter(router); err != nil {
		statusCode := http.StatusInternalServerError
		if err == model_router.ErrEnterpriseFeature {
			statusCode = http.StatusPaymentRequired
		}
		c.JSON(statusCode, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error", Detail: err.Error()}},
		})
		return
	}

	// Emit system event for model router creation
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitModelRouterCreated(router, router.ID, 0)
	}

	c.JSON(http.StatusCreated, gin.H{"data": a.serializeModelRouter(router)})
}

// @Summary Get a model router by ID
// @Description Get details of a model router by its ID (Enterprise only)
// @Tags model-routers
// @Accept json
// @Produce json
// @Param id path int true "Model Router ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Router /model-routers/{id} [get]
// @Security BearerAuth
func (a *API) getModelRouter(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid model router ID"}},
		})
		return
	}

	router, err := a.service.ModelRouterService.GetRouter(uint(id))
	if err != nil {
		statusCode := http.StatusNotFound
		if err == model_router.ErrEnterpriseFeature {
			statusCode = http.StatusPaymentRequired
		}
		c.JSON(statusCode, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeModelRouter(router)})
}

// @Summary Update a model router
// @Description Update an existing model router's information (Enterprise only)
// @Tags model-routers
// @Accept json
// @Produce json
// @Param id path int true "Model Router ID"
// @Param router body ModelRouterInput true "Updated model router information"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 500 {object} ErrorResponse
// @Router /model-routers/{id} [patch]
// @Security BearerAuth
func (a *API) updateModelRouter(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid model router ID"}},
		})
		return
	}

	var input ModelRouterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	router := a.inputToModelRouter(&input)
	router.ID = uint(id)

	if err := a.service.ModelRouterService.UpdateRouter(router); err != nil {
		statusCode := http.StatusInternalServerError
		if err == model_router.ErrEnterpriseFeature {
			statusCode = http.StatusPaymentRequired
		}
		c.JSON(statusCode, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error", Detail: err.Error()}},
		})
		return
	}

	// Emit system event for model router update
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitModelRouterUpdated(router, router.ID, 0)
	}

	// Fetch the updated router to return with all relationships
	updatedRouter, err := a.service.ModelRouterService.GetRouter(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": a.serializeModelRouter(router)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeModelRouter(updatedRouter)})
}

// @Summary Delete a model router
// @Description Delete a model router by ID (Enterprise only)
// @Tags model-routers
// @Accept json
// @Produce json
// @Param id path int true "Model Router ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Router /model-routers/{id} [delete]
// @Security BearerAuth
func (a *API) deleteModelRouter(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid model router ID"}},
		})
		return
	}

	if err := a.service.ModelRouterService.DeleteRouter(uint(id)); err != nil {
		statusCode := http.StatusInternalServerError
		if err == model_router.ErrEnterpriseFeature {
			statusCode = http.StatusPaymentRequired
		}
		c.JSON(statusCode, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error", Detail: err.Error()}},
		})
		return
	}

	// Emit system event for model router deletion
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitModelRouterDeleted(uint(id), 0)
	}

	c.Status(http.StatusNoContent)
}

// @Summary List all model routers
// @Description Get a paginated list of all model routers (Enterprise only)
// @Tags model-routers
// @Accept json
// @Produce json
// @Param page_size query int false "Number of items per page"
// @Param page_number query int false "Page number"
// @Param all query bool false "Return all items without pagination"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 500 {object} ErrorResponse
// @Router /model-routers [get]
// @Security BearerAuth
func (a *API) listModelRouters(c *gin.Context) {
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	pageNumber, _ := strconv.Atoi(c.DefaultQuery("page_number", "1"))
	all := c.Query("all") == "true"

	routers, totalCount, totalPages, err := a.service.ModelRouterService.ListRouters(pageSize, pageNumber, all)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err == model_router.ErrEnterpriseFeature {
			statusCode = http.StatusPaymentRequired
		}
		c.JSON(statusCode, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error", Detail: err.Error()}},
		})
		return
	}

	serialized := make([]map[string]interface{}, len(routers))
	for i, router := range routers {
		serialized[i] = a.serializeModelRouter(&router)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": serialized,
		"meta": gin.H{
			"total_count":  totalCount,
			"total_pages":  totalPages,
			"page_size":    pageSize,
			"page_number":  pageNumber,
		},
	})
}

// @Summary Toggle model router active status
// @Description Enable or disable a model router (Enterprise only)
// @Tags model-routers
// @Accept json
// @Produce json
// @Param id path int true "Model Router ID"
// @Param active body object true "Active status"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Router /model-routers/{id}/toggle [patch]
// @Security BearerAuth
func (a *API) toggleModelRouterActive(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid model router ID"}},
		})
		return
	}

	var input struct {
		Active bool `json:"active"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	if err := a.service.ModelRouterService.ToggleRouterActive(uint(id), input.Active); err != nil {
		statusCode := http.StatusInternalServerError
		if err == model_router.ErrEnterpriseFeature {
			statusCode = http.StatusPaymentRequired
		}
		c.JSON(statusCode, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Error", Detail: err.Error()}},
		})
		return
	}

	// Fetch the updated router
	router, err := a.service.ModelRouterService.GetRouter(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id, "active": input.Active}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": a.serializeModelRouter(router)})
}

// inputToModelRouter converts the API input to a ModelRouter model
func (a *API) inputToModelRouter(input *ModelRouterInput) *models.ModelRouter {
	router := &models.ModelRouter{
		Name:        input.Data.Attributes.Name,
		Slug:        input.Data.Attributes.Slug,
		Description: input.Data.Attributes.Description,
		APICompat:   input.Data.Attributes.APICompat,
		Active:      input.Data.Attributes.Active,
		Namespace:   input.Data.Attributes.Namespace,
		Pools:       make([]*models.ModelPool, len(input.Data.Attributes.Pools)),
	}

	if router.APICompat == "" {
		router.APICompat = string(models.APICompatOpenAI)
	}

	for i, poolInput := range input.Data.Attributes.Pools {
		pool := &models.ModelPool{
			Name:               poolInput.Name,
			ModelPattern:       poolInput.ModelPattern,
			SelectionAlgorithm: models.SelectionAlgorithm(poolInput.SelectionAlgorithm),
			Priority:           poolInput.Priority,
			Vendors:            make([]*models.PoolVendor, len(poolInput.Vendors)),
		}

		if pool.SelectionAlgorithm == "" {
			pool.SelectionAlgorithm = models.SelectionRoundRobin
		}

		for j, vendorInput := range poolInput.Vendors {
			vendor := &models.PoolVendor{
				LLMID:    vendorInput.LLMID,
				Weight:   vendorInput.Weight,
				Active:   vendorInput.Active,
				Mappings: make([]*models.ModelMapping, len(vendorInput.Mappings)),
			}
			if vendor.Weight == 0 {
				vendor.Weight = 1
			}

			// Add vendor-specific mappings
			for k, mappingInput := range vendorInput.Mappings {
				vendor.Mappings[k] = &models.ModelMapping{
					SourceModel: mappingInput.SourceModel,
					TargetModel: mappingInput.TargetModel,
				}
			}

			pool.Vendors[j] = vendor
		}

		router.Pools[i] = pool
	}

	return router
}

// serializeModelRouter converts a ModelRouter to JSON:API format
func (a *API) serializeModelRouter(router *models.ModelRouter) map[string]interface{} {
	pools := make([]map[string]interface{}, len(router.Pools))
	for i, pool := range router.Pools {
		vendors := make([]map[string]interface{}, len(pool.Vendors))
		for j, vendor := range pool.Vendors {
			// Serialize vendor-specific mappings
			mappings := make([]map[string]interface{}, len(vendor.Mappings))
			for k, mapping := range vendor.Mappings {
				mappings[k] = map[string]interface{}{
					"id":           mapping.ID,
					"source_model": mapping.SourceModel,
					"target_model": mapping.TargetModel,
				}
			}

			vendorData := map[string]interface{}{
				"id":       vendor.ID,
				"llm_id":   vendor.LLMID,
				"weight":   vendor.Weight,
				"active":   vendor.Active,
				"mappings": mappings,
			}
			if vendor.LLM != nil {
				vendorData["llm"] = map[string]interface{}{
					"id":     vendor.LLM.ID,
					"name":   vendor.LLM.Name,
					"vendor": vendor.LLM.Vendor,
					"active": vendor.LLM.Active,
				}
			}
			vendors[j] = vendorData
		}

		pools[i] = map[string]interface{}{
			"id":                  pool.ID,
			"name":                pool.Name,
			"model_pattern":       pool.ModelPattern,
			"selection_algorithm": pool.SelectionAlgorithm,
			"priority":            pool.Priority,
			"vendors":             vendors,
		}
	}

	return map[string]interface{}{
		"type": "model-routers",
		"id":   strconv.FormatUint(uint64(router.ID), 10),
		"attributes": map[string]interface{}{
			"name":        router.Name,
			"slug":        router.Slug,
			"description": router.Description,
			"api_compat":  router.APICompat,
			"active":      router.Active,
			"namespace":   router.Namespace,
			"pools":       pools,
			"created_at":  router.CreatedAt,
			"updated_at":  router.UpdatedAt,
		},
	}
}
