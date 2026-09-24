package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// Embedders: reusable embedding configurations that datasources (and
// Semantic Routers) embed with. See features/Embedders.md.

// EmbedderAttributes is an embedder as the API reads and writes it. A linked
// embedder sets llm_id and takes its vendor, endpoint, key and privacy score
// from that LLM; a standalone one sets vendor (its API compatibility),
// endpoint, api_key and privacy_score itself.
type EmbedderAttributes struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	LLMID       *uint  `json:"llm_id"`
	Vendor      string `json:"vendor"`
	Endpoint    string `json:"endpoint"`
	// APIKey: "[redacted]" on update keeps the stored key.
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	PrivacyScore int    `json:"privacy_score"`
}

// EmbedderInput is the JSON:API body of an embedder create or update.
type EmbedderInput struct {
	Data struct {
		Type       string             `json:"type"`
		Attributes EmbedderAttributes `json:"attributes"`
	} `json:"data"`
}

// embedderSortFields are the columns ?sort accepts.
var embedderSortFields = sortable("id", "name", "model", "created_at", "updated_at")

func attributesToEmbedder(in *EmbedderAttributes) *models.Embedder {
	llmID := in.LLMID
	if llmID != nil && *llmID == 0 {
		llmID = nil
	}
	return &models.Embedder{
		Name:         strings.TrimSpace(in.Name),
		Description:  in.Description,
		LLMID:        llmID,
		Vendor:       models.Vendor(strings.TrimSpace(in.Vendor)),
		Endpoint:     strings.TrimSpace(in.Endpoint),
		APIKey:       in.APIKey,
		ModelName:    strings.TrimSpace(in.Model),
		PrivacyScore: in.PrivacyScore,
	}
}

func parseEmbedderID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "Invalid embedder ID")
		return 0, false
	}
	return uint(id), true
}

// @Summary Create an embedder
// @Description Create a reusable embedding configuration, either linked to an LLM (its connection and privacy score come from the LLM) or standalone.
// @Tags embedders
// @Accept json
// @Produce json
// @Param embedder body EmbedderInput true "Embedder"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /embedders [post]
// @Security BearerAuth
func (a *API) createEmbedder(c *gin.Context) {
	var input EmbedderInput
	if err := c.ShouldBindJSON(&input); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if !validatePrivacyScore(c, input.Data.Attributes.PrivacyScore) {
		return
	}
	e, err := a.service.CreateEmbedder(attributesToEmbedder(&input.Data.Attributes), currentUserID(c))
	if err != nil {
		respondEmbedderError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeEmbedder(e)})
}

// @Summary Get an embedder
// @Tags embedders
// @Produce json
// @Param id path int true "Embedder ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /embedders/{id} [get]
// @Security BearerAuth
func (a *API) getEmbedder(c *gin.Context) {
	id, ok := parseEmbedderID(c)
	if !ok {
		return
	}
	e, err := a.service.GetEmbedder(id)
	if err != nil {
		respondEmbedderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeEmbedder(e)})
}

// @Summary Update an embedder
// @Description Replaces the embedder's settings. The model and API compatibility (vendor, or the linked LLM) cannot change while datasources use it (409): their vectors came from the current model.
// @Tags embedders
// @Accept json
// @Produce json
// @Param id path int true "Embedder ID"
// @Param embedder body EmbedderInput true "Embedder"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /embedders/{id} [patch]
// @Security BearerAuth
func (a *API) updateEmbedder(c *gin.Context) {
	id, ok := parseEmbedderID(c)
	if !ok {
		return
	}
	var input EmbedderInput
	if err := c.ShouldBindJSON(&input); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if !validatePrivacyScore(c, input.Data.Attributes.PrivacyScore) {
		return
	}
	e, err := a.service.UpdateEmbedder(id, attributesToEmbedder(&input.Data.Attributes), currentUserID(c))
	if err != nil {
		respondEmbedderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeEmbedder(e)})
}

// @Summary Delete an embedder
// @Description Deletes an embedder that no datasource or Semantic Router uses (409 otherwise, listing them).
// @Tags embedders
// @Param id path int true "Embedder ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /embedders/{id} [delete]
// @Security BearerAuth
func (a *API) deleteEmbedder(c *gin.Context) {
	id, ok := parseEmbedderID(c)
	if !ok {
		return
	}
	if err := a.service.DeleteEmbedder(id, currentUserID(c)); err != nil {
		respondEmbedderError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary List embedders
// @Tags embedders
// @Produce json
// @Param page_size query int false "Number of items per page"
// @Param page query int false "Page number"
// @Param all query bool false "Return all items without pagination"
// @Param search query string false "Case-insensitive substring match on name, description and model"
// @Param sort query string false "Sort field, prefix with - for descending. One of: id, name, model, created_at, updated_at"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse "Unsupported sort field"
// @Router /embedders [get]
// @Security BearerAuth
func (a *API) listEmbedders(c *gin.Context) {
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	pageNumber, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	all := c.Query("all") == "true"
	opts, ok := parseListQuery(c, embedderSortFields)
	if !ok {
		return
	}
	embedders, total, pages, err := a.service.ListEmbedders(pageSize, pageNumber, all, opts)
	if err != nil {
		respondEmbedderError(c, err)
		return
	}
	out := make([]map[string]interface{}, len(embedders))
	for i := range embedders {
		out[i] = serializeEmbedder(&embedders[i])
	}
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{"total_count": total, "total_pages": pages, "page_size": pageSize, "page_number": pageNumber},
	})
}

// @Summary Get embedder dependents
// @Description Lists the datasources and Semantic Routers that embed with the embedder.
// @Tags embedders
// @Produce json
// @Param id path int true "Embedder ID"
// @Success 200 {object} DependentsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /embedders/{id}/dependents [get]
// @Security BearerAuth
func (a *API) getEmbedderDependents(c *gin.Context) {
	a.respondDependents(c, dependentsLookup{
		exists: func(id uint) (bool, error) { return a.rowExists(&models.Embedder{}, id) },
		fetch:  a.service.GetEmbedderDependents,
	})
}

// @Summary List embedding vendors
// @Description The API compatibilities an embedder can use: the vendors whose drivers serve embeddings.
// @Tags embedders
// @Produce json
// @Success 200 {object} VendorListResponse
// @Router /embedders/vendors [get]
// @Security BearerAuth
func (a *API) listEmbedderVendors(c *gin.Context) {
	vendors := a.service.EmbeddingVendors()
	out := make([]string, len(vendors))
	for i, v := range vendors {
		out[i] = string(v)
	}
	c.JSON(http.StatusOK, VendorListResponse{Data: out})
}

// serializeEmbedder is an embedder in JSON:API form. The key is redacted
// (secret references are shown as they are); a linked embedder also shows
// the LLM it inherits from and the vendor and privacy score that come with it.
func serializeEmbedder(e *models.Embedder) map[string]interface{} {
	r := services.RedactedEmbedder(e)
	attrs := map[string]interface{}{
		"name":          e.Name,
		"description":   e.Description,
		"llm_id":        e.LLMID,
		"linked":        e.IsLinked(),
		"vendor":        string(e.Vendor),
		"endpoint":      e.Endpoint,
		"api_key":       r.APIKey,
		"has_api_key":   e.APIKey != "",
		"model":         e.ModelName,
		"privacy_score": e.EffectivePrivacyScore(),
		"created_at":    e.CreatedAt,
		"updated_at":    e.UpdatedAt,
	}
	if e.IsLinked() {
		attrs["privacy_score"] = e.EffectivePrivacyScore()
		if e.LLM != nil {
			attrs["llm_name"] = e.LLM.Name
			attrs["vendor"] = string(e.LLM.Vendor)
		}
	}
	return map[string]interface{}{
		"type":       "embedders",
		"id":         strconv.FormatUint(uint64(e.ID), 10),
		"attributes": attrs,
	}
}

// respondEmbedderError maps an embedder service error to its response.
func respondEmbedderError(c *gin.Context, err error) {
	var inUse *services.EmbedderInUseError
	var locked *services.EmbedderLockedError
	var privacy *services.EmbedderPrivacyError
	switch {
	case errors.Is(err, services.ErrEmbedderNotFound):
		simpleError(c, http.StatusNotFound, "Not Found", "Embedder not found")
	case errors.As(err, &inUse), errors.As(err, &locked), errors.As(err, &privacy):
		simpleError(c, http.StatusConflict, "Conflict", err.Error())
	case errors.Is(err, services.ErrEmbedderInvalid):
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
	default:
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
	}
}
