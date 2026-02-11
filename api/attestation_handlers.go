package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// --- Input types ---

type AttestationTemplateInput struct {
	Data struct {
		Attributes struct {
			Name          string `json:"name"`
			Text          string `json:"text"`
			Required      bool   `json:"required"`
			AppliesToType string `json:"applies_to_type"`
			Active        bool   `json:"active"`
			SortOrder     int    `json:"sort_order"`
		} `json:"attributes"`
	} `json:"data"`
}

// --- Admin attestation template handlers ---

// adminListAttestationTemplates handles GET /api/v1/attestation-templates
func (a *API) adminListAttestationTemplates(c *gin.Context) {
	activeOnly := c.Query("active_only") == "true"

	templates, err := a.service.GetAllAttestationTemplates(activeOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": templates})
}

// adminGetAttestationTemplate handles GET /api/v1/attestation-templates/:id
func (a *API) adminGetAttestationTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid template ID"}},
		})
		return
	}

	template, err := a.service.GetAttestationTemplateByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "attestation template not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": template})
}

// adminCreateAttestationTemplate handles POST /api/v1/attestation-templates
func (a *API) adminCreateAttestationTemplate(c *gin.Context) {
	var input AttestationTemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	attrs := input.Data.Attributes
	if attrs.Name == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "name is required"}},
		})
		return
	}
	if attrs.Text == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "text is required"}},
		})
		return
	}

	template, err := a.service.CreateAttestationTemplate(
		attrs.Name, attrs.Text, attrs.AppliesToType,
		attrs.Required, attrs.Active, attrs.SortOrder,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": template})
}

// adminUpdateAttestationTemplate handles PATCH /api/v1/attestation-templates/:id
func (a *API) adminUpdateAttestationTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid template ID"}},
		})
		return
	}

	var input AttestationTemplateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	attrs := input.Data.Attributes
	template, err := a.service.UpdateAttestationTemplate(
		uint(id), attrs.Name, attrs.Text, attrs.AppliesToType,
		attrs.Required, attrs.Active, attrs.SortOrder,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": template})
}

// adminDeleteAttestationTemplate handles DELETE /api/v1/attestation-templates/:id
func (a *API) adminDeleteAttestationTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid template ID"}},
		})
		return
	}

	if err := a.service.DeleteAttestationTemplate(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "attestation template deleted"})
}
