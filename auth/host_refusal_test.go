package auth_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedHost authenticates every request as one host identity.
type fixedHost struct{}

func (fixedHost) Authenticate(*http.Request) (*services.HostIdentity, error) {
	return &services.HostIdentity{Subject: "host-boss", Email: "boss@host.example.com", Admin: true}, nil
}

// A host-granted admin refused by the licensed admin seats gets the limit's
// message (409 QUOTA_EXCEEDED, as other refusals), not a bare 401, so the
// host can show it. Any other provisioning failure stays a plain 401.
func TestHostSignInRefusedByALicenceLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	svc := services.NewService(db)
	const msg = "Your organisation has reached its licensed limit of 2 users with admin access."
	provisionErr := error(nil)
	a := auth.NewAuthService(&auth.Config{
		DB:       db,
		Service:  svc,
		HostAuth: fixedHost{},
		ProvisionHostUser: func(services.HostIdentity) (*models.User, error) {
			return nil, provisionErr
		},
	}, newMockMailService(), svc, nil)

	r := gin.New()
	r.GET("/api/v1/me", a.AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })
	get := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
		return w
	}

	provisionErr = fmt.Errorf("provision host user: %w", entitlements.NewExceededError(msg))
	w := get()
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var body struct {
		Errors []struct {
			Detail string `json:"detail"`
			Code   string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Errors, 1)
	assert.Equal(t, entitlements.CodeQuotaExceeded, body.Errors[0].Code)
	assert.Equal(t, msg, body.Errors[0].Detail, "only the friendly text, not the wrapping")

	provisionErr = errors.New("db exploded: secret detail")
	w = get()
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.NotContains(t, w.Body.String(), "secret detail")
}
