package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Key sources of a resource signing key.
const (
	signingSourceSecret           = "secret"
	signingSourceVendorCredential = "vendor_credential"
	signingSourceNone             = "none"
)

// ResourceSigningKeyInfo describes one key without revealing it: the kid
// embedded in the IDs it signs and where it comes from.
type ResourceSigningKeyInfo struct {
	KID string `json:"kid"`
	// Source is "secret" (hub-generated) or "vendor_credential" (derived
	// from the LLM's API key, for IDs minted before the LLM had a secret).
	Source string `json:"source"`
}

// ResourceSigningPreviousKey is the key a rotation keeps verifiable.
type ResourceSigningPreviousKey struct {
	ResourceSigningKeyInfo
	ExpiresAt time.Time `json:"expires_at"`
	// Active is false once ExpiresAt has passed: IDs it signed no longer
	// verify (except on the node whose ledger holds the resource).
	Active bool `json:"active"`
}

// ResourceSigningStatus is an LLM's native API resource signing state. The
// secrets themselves are never returned.
type ResourceSigningStatus struct {
	LLMID uint `json:"llm_id"`
	// Source of the current key: "secret", or "vendor_credential" while the
	// LLM has no secret (it gets one at the next Studio start or rotation),
	// or "none" when it has neither.
	Source    string                      `json:"source"`
	KID       string                      `json:"kid,omitempty"`
	RotatedAt *time.Time                  `json:"rotated_at,omitempty"`
	Previous  *ResourceSigningPreviousKey `json:"previous,omitempty"`
}

// ResourceSigningStatusResponse wraps ResourceSigningStatus.
type ResourceSigningStatusResponse struct {
	Data ResourceSigningStatus `json:"data"`
}

// RotateResourceSigningKeyRequest is the optional body of a rotation.
type RotateResourceSigningKeyRequest struct {
	// GracePeriodHours keeps the old key verifiable after the rotation.
	// Omitted: 720 (30 days). 0 revokes it at once. At most 8760.
	GracePeriodHours *int `json:"grace_period_hours"`
}

func resourceSigningStatus(llm *models.LLM, now time.Time) ResourceSigningStatus {
	keys := llm.ResourceSigningKeys()
	credential := secrets.GetValue(llm.APIKey, false)
	st := ResourceSigningStatus{LLMID: llm.ID, Source: signingSourceNone, RotatedAt: keys.RotatedAt}
	if keys.Secret == "" {
		st.RotatedAt = nil
		if k, err := nativeapi.CredentialKey(credential); err == nil {
			st.Source, st.KID = signingSourceVendorCredential, k.KID()
		}
		return st
	}
	if k, err := nativeapi.SecretKey(keys.Secret); err == nil {
		st.Source, st.KID = signingSourceSecret, k.KID()
	}
	if keys.Previous != "" && keys.PreviousExpiresAt != nil {
		prev := &ResourceSigningPreviousKey{ExpiresAt: *keys.PreviousExpiresAt, Active: keys.PreviousActive(now)}
		prev.Source = signingSourceSecret
		if keys.PreviousIsCredential() {
			prev.Source = signingSourceVendorCredential
		}
		resolved := *llm
		resolved.APIKey = credential
		if k, ok, err := nativeapi.PreviousKeyForLLM(&resolved); err == nil && ok {
			prev.KID = k.KID()
		}
		st.Previous = prev
	}
	return st
}

func (a *API) llmIDParam(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		jsonError(c, http.StatusBadRequest, "Bad Request", "Invalid ID format")
		return 0, false
	}
	return uint(id), true
}

// @Summary Get an LLM's resource signing key status
// @Description The kid, source and rotation state of the key that signs the native API resource IDs (for example Anthropic batch IDs) the gateway hands out for this LLM. The key itself is never returned.
// @Tags llms
// @Produce json
// @Param id path int true "LLM ID"
// @Success 200 {object} ResourceSigningStatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /llms/{id}/resource-signing-key [get]
// @Security BearerAuth
func (a *API) getLLMResourceSigningKey(c *gin.Context) {
	id, ok := a.llmIDParam(c)
	if !ok {
		return
	}
	llm, err := a.service.GetLLMByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			jsonError(c, http.StatusNotFound, "Not Found", "LLM not found")
			return
		}
		jsonError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	c.JSON(http.StatusOK, ResourceSigningStatusResponse{Data: resourceSigningStatus(llm, time.Now())})
}

// @Summary Rotate an LLM's resource signing key
// @Description Generates a new secret for signing the LLM's native API resource IDs. IDs signed with the old key keep working for the grace period (default 720 hours; 0 revokes them at once). Edges receive the new key with the next configuration push.
// @Tags llms
// @Accept json
// @Produce json
// @Param id path int true "LLM ID"
// @Param body body RotateResourceSigningKeyRequest false "Grace period"
// @Success 200 {object} ResourceSigningStatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /llms/{id}/resource-signing-key/rotate [post]
// @Security BearerAuth
func (a *API) rotateLLMResourceSigningKey(c *gin.Context) {
	id, ok := a.llmIDParam(c)
	if !ok {
		return
	}
	var req RotateResourceSigningKeyRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			jsonError(c, http.StatusBadRequest, "Bad Request", "Invalid request body: "+err.Error())
			return
		}
	}
	grace := models.DefaultResourceSigningGrace
	if req.GracePeriodHours != nil {
		// Bounded before the multiplication, which would overflow.
		h := *req.GracePeriodHours
		if h < 0 || h > int(models.MaxResourceSigningGrace/time.Hour) {
			jsonError(c, http.StatusBadRequest, "Bad Request",
				"grace_period_hours must be between 0 and "+strconv.Itoa(int(models.MaxResourceSigningGrace/time.Hour)))
			return
		}
		grace = time.Duration(h) * time.Hour
	}

	llm, err := a.service.RotateLLMResourceSigningSecret(id, grace, currentUserID(c))
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		jsonError(c, http.StatusNotFound, "Not Found", "LLM not found")
		return
	case errors.Is(err, models.ErrResourceSigningGrace):
		jsonError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	case errors.Is(err, models.ErrResourceSigningConflict):
		jsonError(c, http.StatusConflict, "Conflict", "the signing key was rotated concurrently; reload and try again")
		return
	case err != nil:
		jsonError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	if a.proxy != nil && llm.Active {
		a.proxy.Reload()
	}
	c.JSON(http.StatusOK, ResourceSigningStatusResponse{Data: resourceSigningStatus(llm, time.Now())})
}
