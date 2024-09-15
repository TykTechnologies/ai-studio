package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// @Summary Create a new model price
// @Description Create a new model price with the provided information
// @Tags model-prices
// @Accept json
// @Produce json
// @Param modelPrice body ModelPriceInput true "Model Price information"
// @Success 201 {object} ModelPriceResponse
// @Failure 400 {object} ErrorResponse
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

	modelPrice, err := a.service.CreateModelPrice(
		input.Data.Attributes.ModelName,
		input.Data.Attributes.Vendor,
		input.Data.Attributes.CPT,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
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
// @Description Update an existing model price's information
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
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	modelPrice, err := a.service.UpdateModelPrice(
		uint(id),
		input.Data.Attributes.ModelName,
		input.Data.Attributes.Vendor,
		input.Data.Attributes.CPT,
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

	c.Status(http.StatusNoContent)
}

// @Summary Get all model prices
// @Description Get a list of all model prices
// @Tags model-prices
// @Accept json
// @Produce json
// @Success 200 {array} ModelPriceResponse
// @Failure 500 {object} ErrorResponse
// @Router /model-prices [get]
// @Security BearerAuth
func (a *API) getAllModelPrices(c *gin.Context) {
	modelPrices, err := a.service.GetAllModelPrices()
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
			ModelName string  `json:"model_name"`
			Vendor    string  `json:"vendor"`
			CPT       float64 `json:"cpt"`
		}{
			ModelName: mp.ModelName,
			Vendor:    mp.Vendor,
			CPT:       mp.CPT,
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
