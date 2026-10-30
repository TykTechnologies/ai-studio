package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
)

// Trusted prompt templates of an app (features/TrustedPromptTemplates.md).
// Anyone who may write the app proposes and edits; only a full administrator
// approves (the route's permission), since approving exempts the bytes from
// guardrail scoring.

// AppPromptTemplateInput creates or changes a template. On PATCH omitted
// fields are left as they are.
type AppPromptTemplateInput struct {
	Name             string               `json:"name"`
	Placement        *templates.Placement `json:"placement"`
	Parts            []templates.Part     `json:"parts"`
	ExpiresAfterDays *int                 `json:"expires_after_days"`
}

// AppPromptTemplateResponse is one template version.
type AppPromptTemplateResponse struct {
	ID               uint                `json:"id"`
	AppID            uint                `json:"app_id"`
	Name             string              `json:"name"`
	Version          int                 `json:"version"`
	Ref              string              `json:"ref"`
	Placement        templates.Placement `json:"placement"`
	Parts            []templates.Part    `json:"parts"`
	Status           string              `json:"status"`
	CreatedBy        uint                `json:"created_by"`
	ApprovedBy       *uint               `json:"approved_by"`
	ApprovedAt       *time.Time          `json:"approved_at"`
	ExpiresAfterDays int                 `json:"expires_after_days"`
	LastMatchedAt    *time.Time          `json:"last_matched_at"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

func serializeAppPromptTemplate(t *models.AppPromptTemplate) AppPromptTemplateResponse {
	return AppPromptTemplateResponse{
		ID: t.ID, AppID: t.AppID, Name: t.Name, Version: t.Version, Ref: t.Matcher().Ref(),
		Placement: t.Placement(), Parts: t.Parts, Status: t.Status,
		CreatedBy: t.CreatedBy, ApprovedBy: t.ApprovedBy, ApprovedAt: t.ApprovedAt,
		ExpiresAfterDays: t.ExpiresAfterDays, LastMatchedAt: t.LastMatchedAt,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

// promptTemplateIDs reads the app and template ids of the route.
func promptTemplateIDs(c *gin.Context, withTemplate bool) (appID, templateID uint, ok bool) {
	a, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "Invalid app ID"))
		return 0, 0, false
	}
	if !withTemplate {
		return uint(a), 0, true
	}
	t, err := strconv.ParseUint(c.Param("template_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "Invalid template ID"))
		return 0, 0, false
	}
	return uint(a), uint(t), true
}

// promptTemplateError answers a service error.
func promptTemplateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, errorResponse("Not Found", "App or prompt template not found"))
	case errors.Is(err, services.ErrInvalidPromptTemplate):
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", err.Error()))
	case errors.Is(err, services.ErrPromptTemplateConflict):
		c.JSON(http.StatusConflict, errorResponse("Conflict", err.Error()))
	default:
		c.JSON(http.StatusInternalServerError, errorResponse("Internal Server Error", "Could not process the prompt template"))
	}
}

// @Summary List an app's prompt templates
// @Description Lists the trusted prompt template versions of an app, optionally by status (proposed, approved, retired).
// @Tags apps
// @Produce json
// @Param id path int true "App ID"
// @Param status query string false "Status filter"
// @Success 200 {array} AppPromptTemplateResponse
// @Router /apps/{id}/prompt-templates [get]
// @Security BearerAuth
func (a *API) listAppPromptTemplates(c *gin.Context) {
	appID, _, ok := promptTemplateIDs(c, false)
	if !ok {
		return
	}
	list, err := a.service.ListAppPromptTemplates(appID, c.Query("status"))
	if err != nil {
		promptTemplateError(c, err)
		return
	}
	out := make([]AppPromptTemplateResponse, len(list))
	for i := range list {
		out[i] = serializeAppPromptTemplate(&list[i])
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// @Summary Get an app's prompt template
// @Tags apps
// @Produce json
// @Param id path int true "App ID"
// @Param template_id path int true "Template ID"
// @Success 200 {object} AppPromptTemplateResponse
// @Router /apps/{id}/prompt-templates/{template_id} [get]
// @Security BearerAuth
func (a *API) getAppPromptTemplate(c *gin.Context) {
	appID, id, ok := promptTemplateIDs(c, true)
	if !ok {
		return
	}
	t, err := a.service.GetAppPromptTemplate(appID, id)
	if err != nil {
		promptTemplateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeAppPromptTemplate(t)})
}

// @Summary Propose a prompt template
// @Description Proposes a trusted prompt template for an app: version 1 of a new name, or the next version of an existing one. It has no effect until an administrator approves it.
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param template body AppPromptTemplateInput true "Template"
// @Success 201 {object} AppPromptTemplateResponse
// @Router /apps/{id}/prompt-templates [post]
// @Security BearerAuth
func (a *API) createAppPromptTemplate(c *gin.Context) {
	appID, _, ok := promptTemplateIDs(c, false)
	if !ok {
		return
	}
	var in AppPromptTemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", err.Error()))
		return
	}
	t, err := a.service.CreateAppPromptTemplate(appID, currentUserID(c), services.PromptTemplateSpec(in))
	if err != nil {
		promptTemplateError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeAppPromptTemplate(t)})
}

// @Summary Change a prompt template
// @Description Changes a template. A change to the parts or placement of an approved or retired version creates the next version, proposed, which needs approving again; the response is that version.
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param template_id path int true "Template ID"
// @Param template body AppPromptTemplateInput true "Changes"
// @Success 200 {object} AppPromptTemplateResponse
// @Router /apps/{id}/prompt-templates/{template_id} [patch]
// @Security BearerAuth
func (a *API) updateAppPromptTemplate(c *gin.Context) {
	appID, id, ok := promptTemplateIDs(c, true)
	if !ok {
		return
	}
	var in AppPromptTemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", err.Error()))
		return
	}
	t, err := a.service.UpdateAppPromptTemplate(appID, id, currentUserID(c), services.PromptTemplateSpec(in))
	if err != nil {
		promptTemplateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeAppPromptTemplate(t)})
}

// @Summary Delete a prompt template version
// @Tags apps
// @Param id path int true "App ID"
// @Param template_id path int true "Template ID"
// @Success 204
// @Router /apps/{id}/prompt-templates/{template_id} [delete]
// @Security BearerAuth
func (a *API) deleteAppPromptTemplate(c *gin.Context) {
	appID, id, ok := promptTemplateIDs(c, true)
	if !ok {
		return
	}
	if err := a.service.DeleteAppPromptTemplate(appID, id); err != nil {
		promptTemplateError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary Approve a prompt template
// @Description Approves a proposed version (administrators only). Request guardrails then leave its constants unscored. Other approved versions of the same name are retired; an app has at most 10 approved templates.
// @Tags apps
// @Produce json
// @Param id path int true "App ID"
// @Param template_id path int true "Template ID"
// @Success 200 {object} AppPromptTemplateResponse
// @Failure 409 {object} ErrorResponse
// @Router /apps/{id}/prompt-templates/{template_id}/approve [post]
// @Security BearerAuth
func (a *API) approveAppPromptTemplate(c *gin.Context) {
	appID, id, ok := promptTemplateIDs(c, true)
	if !ok {
		return
	}
	t, err := a.service.ApproveAppPromptTemplate(appID, id, currentUserID(c))
	if err != nil {
		promptTemplateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeAppPromptTemplate(t)})
}

// @Summary Retire a prompt template
// @Description Retires a version: it is kept for the record and never matched again.
// @Tags apps
// @Produce json
// @Param id path int true "App ID"
// @Param template_id path int true "Template ID"
// @Success 200 {object} AppPromptTemplateResponse
// @Router /apps/{id}/prompt-templates/{template_id}/retire [post]
// @Security BearerAuth
func (a *API) retireAppPromptTemplate(c *gin.Context) {
	appID, id, ok := promptTemplateIDs(c, true)
	if !ok {
		return
	}
	t, err := a.service.RetireAppPromptTemplate(appID, id)
	if err != nil {
		promptTemplateError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeAppPromptTemplate(t)})
}
