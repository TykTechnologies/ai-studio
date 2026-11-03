package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gin-gonic/gin"
)

// The guardrail capability of an app (features/GuardrailExceptions.md): what
// a full administrator attests the app can do, which bounds how far guardrail
// exceptions may soften a verdict for it. Studio suggests a value; only a
// full administrator sets one (the route's permission).

// AppGuardrailCapabilityResponse is an app's attested capability and
// Studio's suggestion.
type AppGuardrailCapabilityResponse struct {
	// Value is the attested capability: read_only, read_tools, acting, or ""
	// when nobody has attested one (treated as acting).
	Value string `json:"value"`
	// Effective is the capability the guardrail floors apply ("" is acting).
	Effective  string   `json:"effective"`
	Suggestion string   `json:"suggestion"`
	Reasons    []string `json:"reasons"`
}

// AppGuardrailCapabilityInput sets an app's capability.
type AppGuardrailCapabilityInput struct {
	// Value is read_only, read_tools or acting; "" clears the attestation.
	Value *string `json:"value"`
}

func appGuardrailCapabilityID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "Invalid app ID"))
		return 0, false
	}
	return uint(id), true
}

func appGuardrailCapabilityError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, errorResponse("Not Found", "App not found"))
	case errors.Is(err, services.ErrInvalidGuardrailCapability):
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", err.Error()))
	default:
		c.JSON(http.StatusInternalServerError, errorResponse("Internal Server Error", "Could not process the guardrail capability"))
	}
}

func appGuardrailCapabilityResponse(value string, s services.CapabilitySuggestion) AppGuardrailCapabilityResponse {
	reasons := s.Reasons
	if reasons == nil {
		reasons = []string{}
	}
	return AppGuardrailCapabilityResponse{
		Value:      value,
		Effective:  string(guardrails.Capability(value).Effective()),
		Suggestion: string(s.Suggestion),
		Reasons:    reasons,
	}
}

// @Summary Get an app's guardrail capability
// @Description The capability a full administrator attested for the app (value, "" when unattested and treated as acting) and the one Studio suggests from its tools, MCP servers and datasources, with the reasons.
// @Tags apps
// @Produce json
// @Param id path int true "App ID"
// @Success 200 {object} AppGuardrailCapabilityResponse
// @Router /apps/{id}/guardrail-capability [get]
// @Security BearerAuth
func (a *API) getAppGuardrailCapability(c *gin.Context) {
	id, ok := appGuardrailCapabilityID(c)
	if !ok {
		return
	}
	app, err := a.service.GetAppByID(id)
	if err != nil {
		appGuardrailCapabilityError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": appGuardrailCapabilityResponse(app.GuardrailCapability, services.SuggestAppCapability(app))})
}

// @Summary Set an app's guardrail capability
// @Description Attests what the app can do (read_only, read_tools or acting; "" clears it). Full administrators only.
// @Tags apps
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param capability body AppGuardrailCapabilityInput true "Capability"
// @Success 200 {object} AppGuardrailCapabilityResponse
// @Router /apps/{id}/guardrail-capability [put]
// @Security BearerAuth
func (a *API) setAppGuardrailCapability(c *gin.Context) {
	id, ok := appGuardrailCapabilityID(c)
	if !ok {
		return
	}
	var in AppGuardrailCapabilityInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", err.Error()))
		return
	}
	if in.Value == nil {
		c.JSON(http.StatusBadRequest, errorResponse("Bad Request", "value is required"))
		return
	}
	app, err := a.service.SetAppGuardrailCapability(id, *in.Value, currentUserID(c))
	if err != nil {
		appGuardrailCapabilityError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": appGuardrailCapabilityResponse(app.GuardrailCapability, services.SuggestAppCapability(app))})
}
