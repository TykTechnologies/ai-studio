package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// validatePricePlausibility rejects prices that are almost certainly per-million
// figures submitted into per-token fields. Without this the mistake is silent,
// and every cost figure, budget calculation and budget alert downstream is
// wrong by a factor of a million.
func validatePricePlausibility(attrs struct {
	CPT          float64
	CPIT         float64
	CacheWritePT float64
	CacheReadPT  float64
}) (bool, string) {
	for _, f := range []struct {
		name  string
		value float64
	}{
		{"cpt (price per output token)", attrs.CPT},
		{"cpit (price per input token)", attrs.CPIT},
		{"cache_write_pt", attrs.CacheWritePT},
		{"cache_read_pt", attrs.CacheReadPT},
	} {
		if bad, msg := models.ImplausiblePerTokenPrice(f.name, f.value); bad {
			return true, msg
		}
	}
	return false, ""
}

// @Summary Create a new model price
// @Description Create a new model price with the provided information. Traffic auto-creates an unpriced (all zero) price for every model it sees; when the model and vendor have one, it is filled in and 200 is returned. When they already have a priced row, 409 names it.
// @Tags model-prices
// @Accept json
// @Produce json
// @Param modelPrice body ModelPriceInput true "Model Price information"
// @Success 200 {object} ModelPriceResponse
// @Success 201 {object} ModelPriceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /model-prices [post]
// @Security BearerAuth
func (a *API) createModelPrice(c *gin.Context) {
	var input ModelPriceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	if input.Data.Attributes.ModelName == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Model name is required"}},
		})
		return
	}

	if input.Data.Attributes.Vendor == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Vendor is required"}},
		})
		return
	}

	if input.Data.Attributes.CPT < 0.0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "CPT must be greater than or equal to 0.0"}},
		})
		return
	}

	if bad, detail := validatePricePlausibility(struct {
		CPT          float64
		CPIT         float64
		CacheWritePT float64
		CacheReadPT  float64
	}{
		input.Data.Attributes.CPT,
		input.Data.Attributes.CPIT,
		input.Data.Attributes.CacheWritePT,
		input.Data.Attributes.CacheReadPT,
	}); bad {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Implausible Price", Detail: detail}},
		})
		return
	}

	modelPrice, filled, err := a.service.CreateOrFillModelPrice(
		input.Data.Attributes.ModelName,
		input.Data.Attributes.Vendor,
		input.Data.Attributes.CPT,
		input.Data.Attributes.CPIT,
		input.Data.Attributes.CacheWritePT,
		input.Data.Attributes.CacheReadPT,
		input.Data.Attributes.Currency,
	)
	var exists *services.ModelPriceExistsError
	if errors.As(err, &exists) {
		c.JSON(http.StatusConflict, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Conflict", Detail: exists.Error()}},
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// The model's unpriced row (auto-created by traffic) was filled in.
	if filled {
		if a.service.SystemEvents != nil {
			a.service.SystemEvents.EmitModelPriceUpdated(modelPrice, modelPrice.ID, 0)
		}
		c.JSON(http.StatusOK, gin.H{"data": serializeModelPrice(modelPrice)})
		return
	}

	// Emit system event for model price creation
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitModelPriceCreated(modelPrice, modelPrice.ID, 0)
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeModelPrice(modelPrice)})
}

// @Summary Get a model price by ID
// @Description Get details of a model price by its ID
// @Tags model-prices
// @Accept json
// @Produce json
// @Param id path int true "Model Price ID"
// @Success 200 {object} ModelPriceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /model-prices/{id} [get]
// @Security BearerAuth
func (a *API) getModelPrice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid model price ID"}},
		})
		return
	}

	modelPrice, err := a.service.GetModelPriceByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Model price not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeModelPrice(modelPrice)})
}

// @Summary Update a model price
// @Description Update an existing model price's information without recalculating historical costs
// @Tags model-prices
// @Accept json
// @Produce json
// @Param id path int true "Model Price ID"
// @Param modelPrice body ModelPriceInput true "Updated model price information"
// @Success 200 {object} ModelPriceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /model-prices/{id} [patch]
// @Security BearerAuth
func (a *API) updateModelPrice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid model price ID"}},
		})
		return
	}

	var input ModelPriceInput
	if err := c.ShouldBindBodyWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}
	if !a.mergeModelPricePatch(c, uint(id), &input) {
		return
	}

	if bad, detail := validatePricePlausibility(struct {
		CPT          float64
		CPIT         float64
		CacheWritePT float64
		CacheReadPT  float64
	}{
		input.Data.Attributes.CPT,
		input.Data.Attributes.CPIT,
		input.Data.Attributes.CacheWritePT,
		input.Data.Attributes.CacheReadPT,
	}); bad {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Implausible Price", Detail: detail}},
		})
		return
	}

	modelPrice, err := a.service.UpdateModelPrice(
		uint(id),
		input.Data.Attributes.ModelName,
		input.Data.Attributes.Vendor,
		input.Data.Attributes.CPT,
		input.Data.Attributes.CPIT,
		input.Data.Attributes.CacheWritePT,
		input.Data.Attributes.CacheReadPT,
		input.Data.Attributes.Currency,
	)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Model price not found"}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// Emit system event for model price update
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitModelPriceUpdated(modelPrice, modelPrice.ID, 0)
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeModelPrice(modelPrice)})
}

// @Summary Delete a model price
// @Description Delete a model price by its ID
// @Tags model-prices
// @Accept json
// @Produce json
// @Param id path int true "Model Price ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /model-prices/{id} [delete]
// @Security BearerAuth
func (a *API) deleteModelPrice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid model price ID"}},
		})
		return
	}

	err = a.service.DeleteModelPrice(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// Emit system event for model price deletion
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitModelPriceDeleted(uint(id), 0)
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get all model prices
// @Description Get a list of all model prices
// @Tags model-prices
// @Accept json
// @Produce json
// @Param search query string false "Case-insensitive substring match on name and description fields"
// @Param sort query string false "Sort field, prefix with - for descending. One of: id, name (model_name), vendor, created_at, updated_at"
// @Success 200 {array} ModelPriceResponse
// @Failure 500 {object} ErrorResponse
// @Router /model-prices [get]
// @Security BearerAuth
func (a *API) getAllModelPrices(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)
	opts, ok := parseListQuery(c, modelPriceSortFields)
	if !ok {
		return
	}

	modelPrices, totalCount, totalPages, err := a.service.GetAllModelPrices(pageSize, pageNumber, all, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	unpriced, err := a.service.CountUnpricedHistory(modelPrices)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}
	data := serializeModelPrices(modelPrices)
	for i := range data {
		data[i].Meta = &ModelPriceMeta{UnpricedRows: unpriced[modelPrices[i].ID]}
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// @Summary Backfill a model price's unpriced history
// @Description Price the chat records this price prices that were recorded at no cost (traffic from before the model had a price), at the current price, optionally only within a date range (inclusive dates, UTC). Records that have a cost are left alone; to reprice those, use PATCH /model-prices/{id}/recalculate.
// @Tags model-prices
// @Accept json
// @Produce json
// @Param id path int true "Model Price ID"
// @Param range body BackfillModelPriceInput false "Optional date range"
// @Success 200 {object} BackfillModelPriceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /model-prices/{id}/backfill [post]
// @Security BearerAuth
func (a *API) backfillModelPriceHistory(c *gin.Context) {
	fail := func(status int, title, detail string) {
		c.JSON(status, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: title, Detail: detail}},
		})
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		fail(http.StatusBadRequest, "Bad Request", "Invalid model price ID")
		return
	}

	var input BackfillModelPriceInput
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&input); err != nil {
			fail(http.StatusBadRequest, "Bad Request", err.Error())
			return
		}
	}
	// The range is inclusive dates: to is the end of its day.
	var from, to *time.Time
	if input.From != "" {
		d, err := time.Parse(time.DateOnly, input.From)
		if err != nil {
			fail(http.StatusBadRequest, "Bad Request", "from must be a date (YYYY-MM-DD)")
			return
		}
		from = &d
	}
	if input.To != "" {
		d, err := time.Parse(time.DateOnly, input.To)
		if err != nil {
			fail(http.StatusBadRequest, "Bad Request", "to must be a date (YYYY-MM-DD)")
			return
		}
		d = d.AddDate(0, 0, 1)
		to = &d
	}
	if from != nil && to != nil && !from.Before(*to) {
		fail(http.StatusBadRequest, "Bad Request", "from must not be after to")
		return
	}

	rows, err := a.service.BackfillUnpricedHistory(uint(id), from, to)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fail(http.StatusNotFound, "Not Found", "Model price not found")
		return
	}
	if err != nil {
		fail(http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	modelPrice, err := a.service.GetModelPriceByID(uint(id))
	if err != nil {
		fail(http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	// kq3b.3/kq3b.8 (dev-2): budget tracker invalidation and the price event
	// for changed history go here, as after a recalculation.

	var resp BackfillModelPriceResponse
	resp.Data.RowsPriced = rows
	resp.Data.ModelPrice = serializeModelPrice(modelPrice)
	c.JSON(http.StatusOK, resp)
}

// @Summary Get model prices by vendor
// @Description Get a list of model prices for a specific vendor
// @Tags model-prices
// @Accept json
// @Produce json
// @Param vendor query string true "Vendor name"
// @Success 200 {array} ModelPriceResponse
// @Failure 500 {object} ErrorResponse
// @Router /model-prices/by-vendor [get]
// @Security BearerAuth
func (a *API) getModelPricesByVendor(c *gin.Context) {
	vendor := c.Query("vendor")
	modelPrices, err := a.service.GetModelPricesByVendor(vendor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeModelPrices(modelPrices)})
}

func serializeModelPrice(mp *models.ModelPrice) ModelPriceResponse {
	return ModelPriceResponse{
		Type: "model-prices",
		ID:   strconv.FormatUint(uint64(mp.ID), 10),
		Attributes: struct {
			ModelName    string  `json:"model_name"`
			Vendor       string  `json:"vendor"`
			CPT          float64 `json:"cpt"`
			CPIT         float64 `json:"cpit"`
			CacheWritePT float64 `json:"cache_write_pt"`
			CacheReadPT  float64 `json:"cache_read_pt"`
			Currency     string  `json:"currency"`
		}{
			ModelName:    mp.ModelName,
			Vendor:       mp.Vendor,
			CPT:          mp.CPT,
			CPIT:         mp.CPIT,
			CacheWritePT: mp.CacheWritePT,
			CacheReadPT:  mp.CacheReadPT,
			Currency:     mp.Currency,
		},
	}
}

func serializeModelPrices(mps models.ModelPrices) []ModelPriceResponse {
	result := make([]ModelPriceResponse, len(mps))
	for i, mp := range mps {
		result[i] = serializeModelPrice(&mp)
	}
	return result
}

// @Summary Update a model price and recalculate historical costs
// @Description Update an existing model price's information and recalculate all historical chat record costs
// @Tags model-prices
// @Accept json
// @Produce json
// @Param id path int true "Model Price ID"
// @Param modelPrice body ModelPriceInput true "Updated model price information"
// @Success 200 {object} ModelPriceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /model-prices/{id}/recalculate [patch]
// @Security BearerAuth
func (a *API) updateModelPriceAndRecalculate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid model price ID"}},
		})
		return
	}

	var input ModelPriceInput
	if err := c.ShouldBindBodyWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}
	if !a.mergeModelPricePatch(c, uint(id), &input) {
		return
	}

	modelPrice, err := a.service.UpdateModelPriceAndRecalculate(
		uint(id),
		input.Data.Attributes.ModelName,
		input.Data.Attributes.Vendor,
		input.Data.Attributes.CPT,
		input.Data.Attributes.CPIT,
		input.Data.Attributes.CacheWritePT,
		input.Data.Attributes.CacheReadPT,
		input.Data.Attributes.Currency,
	)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Model price not found"}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// The same event as a standard update: edges are flagged for a config
	// push and plugins hear of the new price.
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitModelPriceUpdated(modelPrice, modelPrice.ID, 0)
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeModelPrice(modelPrice)})
}

// @Summary Get or create a model price by name
// @Description Get a model price by its name, creating it with default values if it doesn't exist
// @Tags model-prices
// @Accept json
// @Produce json
// @Param model_name query string true "Model name"
// @Success 200 {object} ModelPriceResponse
// @Failure 500 {object} ErrorResponse
// @Router /model-prices/by-name [get]
// @Security BearerAuth
func (a *API) getOrCreateModelPriceByName(c *gin.Context) {
	modelName := c.Query("model_name")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Model name is required"}},
		})
		return
	}

	modelPrice, err := a.service.GetOrCreateModelPriceByName(modelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeModelPrice(modelPrice)})
}
