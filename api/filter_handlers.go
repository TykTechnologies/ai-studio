package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// @Summary Create a new filter
// @Description Create a new filter with the given input data
// @Tags filters
// @Accept json
// @Produce json
// @Param input body FilterInput true "Filter input"
// @Success 201 {object} FilterResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /filters [post]
func (a *API) createFilter(c *gin.Context) {
	// Check if enterprise features are enabled
	if !config.IsEnterprise() {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Enterprise Feature", "Filter scripting is an Enterprise feature. Visit https://tyk.io/enterprise for more information."}}})
		return
	}

	var input FilterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", err.Error()}}})
		return
	}

	if input.Data.Attributes.Name == "" || input.Data.Attributes.Description == "" || len(input.Data.Attributes.Script) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Name, description, and script are required"}}})
		return
	}

	filter, err := a.service.CreateFilter(
		input.Data.Attributes.Name,
		input.Data.Attributes.Description,
		input.Data.Attributes.Script,
		input.Data.Attributes.ResponseFilter,
		input.Data.Attributes.Namespace,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.JSON(http.StatusCreated, toFilterResponse(filter))
}

// @Summary Get a filter by ID
// @Description Get a filter's details by its ID
// @Tags filters
// @Produce json
// @Param id path int true "Filter ID"
// @Success 200 {object} FilterResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /filters/{id} [get]
func (a *API) getFilter(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	filter, err := a.service.GetFilterByID(uint(id))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{"Not Found", "Filter not found"}}})
			return
		}

		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Not Found", err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, toFilterResponse(filter))
}

// @Summary Update a filter
// @Description Update an existing filter's details
// @Tags filters
// @Accept json
// @Produce json
// @Param id path int true "Filter ID"
// @Param input body FilterInput true "Updated filter input"
// @Success 200 {object} FilterResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /filters/{id} [patch]
func (a *API) updateFilter(c *gin.Context) {
	// Check if enterprise features are enabled
	if !config.IsEnterprise() {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Enterprise Feature", "Filter scripting is an Enterprise feature. Visit https://tyk.io/enterprise for more information."}}})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	var input FilterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", err.Error()}}})
		return
	}

	filter, err := a.service.UpdateFilter(
		uint(id),
		input.Data.Attributes.Name,
		input.Data.Attributes.Description,
		input.Data.Attributes.Script,
		input.Data.Attributes.ResponseFilter,
		input.Data.Attributes.Namespace,
	)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{"Not Found", "Filter not found"}}})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.JSON(http.StatusOK, toFilterResponse(filter))
}

// @Summary Delete a filter
// @Description Delete a filter by its ID
// @Tags filters
// @Produce json
// @Param id path int true "Filter ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /filters/{id} [delete]
func (a *API) deleteFilter(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Bad Request", "Invalid ID format"}}})
		return
	}

	err = a.service.DeleteFilter(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List all filters
// @Description Get a list of all filters
// @Tags filters
// @Produce json
// @Success 200 {array} FilterResponse
// @Failure 500 {object} ErrorResponse
// @Router /filters [get]
func (a *API) listFilters(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	filters, totalCount, totalPages, err := a.service.GetAllFilters(pageSize, pageNumber, all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{"Internal Server Error", err.Error()}}})
		return
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, toFilterResponses(filters))
}

func toFilterResponse(filter *models.Filter) FilterResponse {
	return FilterResponse{
		Type: "filter",
		ID:   strconv.FormatUint(uint64(filter.ID), 10),
		Attributes: struct {
			Name           string `json:"name"`
			Description    string `json:"description"`
			Script         []byte `json:"script"`
			ResponseFilter bool   `json:"response_filter"`
			Namespace      string `json:"namespace"`
		}{
			Name:           filter.Name,
			Description:    filter.Description,
			Script:         filter.Script,
			ResponseFilter: filter.ResponseFilter,
			Namespace:      filter.Namespace,
		},
	}
}

func toFilterResponses(filters []models.Filter) []FilterResponse {
	responses := make([]FilterResponse, len(filters))
	for i, filter := range filters {
		responses[i] = toFilterResponse(&filter)
	}
	return responses
}
