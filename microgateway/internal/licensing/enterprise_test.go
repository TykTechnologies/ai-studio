//go:build enterprise
// +build enterprise

package licensing

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/TykTechnologies/ai-studio-enterprise/v2/licensing/verify"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Enterprise microgateway verifies licences with the shared verifier:
// a correctly signed, unexpired licence without the AI Studio SKU fails its
// boot validation with the SKU error, and a v2 AI Studio licence passes.
// (Start itself exits the process on failure, so validateLicense is tested.)
func TestMicrogatewayLicenceNeedsAIStudioSKU(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	newSvc := func(token string) *enterpriseService {
		s := NewService(Config{LicenseKey: token, ValidityCheckPeriod: time.Hour}).(*enterpriseService)
		s.publicKey = &priv.PublicKey
		return s
	}
	sign := func(c jwt.MapClaims) string {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodRS256, c).SignedString(priv)
		require.NoError(t, err)
		return tok
	}
	exp := time.Now().Add(48 * time.Hour)
	base := func() jwt.MapClaims {
		return jwt.MapClaims{
			verify.ClaimExp:     float64(exp.Unix()),
			verify.ClaimVersion: verify.ClaimsVersion,
			verify.ClaimSKUs:    []interface{}{verify.SKU},
			verify.ClaimProducts: map[string]interface{}{verify.SKU: map[string]interface{}{
				verify.ClaimLimits: map[string]interface{}{verify.LimitNamespaces: 1},
			}},
		}
	}

	t.Run("v2 AI Studio licence is valid", func(t *testing.T) {
		s := newSvc(sign(base()))
		require.NoError(t, s.validateLicense())
		assert.True(t, s.IsValid())
		assert.InDelta(t, 1, s.DaysLeft(), 1)
	})

	t.Run("another product's licence fails with the SKU error", func(t *testing.T) {
		c := base()
		c[verify.ClaimSKUs] = []interface{}{"tyk-dashboard"}
		s := newSvc(sign(c))
		err := s.validateLicense()
		require.Error(t, err)
		assert.True(t, errors.Is(err, verify.ErrMissingSKU), "got %v", err)
		assert.Contains(t, err.Error(), "license does not include the Tyk AI Studio SKU")
		assert.False(t, s.IsValid())
	})

	t.Run("old v1 scope licence fails with the SKU error", func(t *testing.T) {
		s := newSvc(sign(jwt.MapClaims{
			verify.ClaimExp: float64(exp.Unix()),
			"scope":         "feature_portal,feature_chat,feature_gateway",
			"dat":           map[string]interface{}{"feature_gateway": true},
		}))
		assert.ErrorIs(t, s.validateLicense(), verify.ErrMissingSKU)
	})

	t.Run("expired licence fails", func(t *testing.T) {
		c := base()
		c[verify.ClaimExp] = float64(time.Now().Add(-time.Hour).Unix())
		assert.ErrorIs(t, newSvc(sign(c)).validateLicense(), verify.ErrExpired)
	})

	t.Run("licence signed by another key fails", func(t *testing.T) {
		other, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		tok, err := jwt.NewWithClaims(jwt.SigningMethodRS256, base()).SignedString(other)
		require.NoError(t, err)
		assert.ErrorIs(t, newSvc(tok).validateLicense(), verify.ErrInvalidSignature)
	})

	t.Run("the built-in key is the shared verifier's", func(t *testing.T) {
		s := NewService(Config{}).(*enterpriseService)
		want, err := verify.DefaultPublicKey()
		require.NoError(t, err)
		assert.True(t, want.Equal(s.publicKey))
	})
}
