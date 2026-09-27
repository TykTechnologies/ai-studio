package services

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	coresvc "github.com/TykTechnologies/midsommar/v2/services"
)

// The gateway answers 401 only when the control plane says a token is
// invalid. Everything that stops it asking (no client, a transport or hub
// error, an accepted App that is not in SQLite yet) is "cannot check right
// now", which the proxy turns into a retryable 503. Before this every failure
// read as "invalid token", so a hub outage looked to clients like a revoked key.
func TestHybridValidateAPIToken_ClassifiesFailures(t *testing.T) {
	unavailable := []struct {
		name   string
		client interface{}
	}{
		{"no edge client", nil},
		{"hub unreachable", &fakeEdgeClient{err: status.Error(codes.Unavailable, "connection refused")}},
		{"hub database fault", &fakeEdgeClient{err: status.Error(codes.Internal, "token validation failed")}},
		{"hub too slow", &fakeEdgeClient{err: status.Error(codes.DeadlineExceeded, "context deadline exceeded")}},
		{"accepted App missing locally", &fakeEdgeClient{resp: &pb.TokenValidationResponse{Valid: true, AppId: 999}}},
	}
	for _, tc := range unavailable {
		t.Run(tc.name, func(t *testing.T) {
			h := newStaleGraceService(t, 0)
			if tc.client != nil {
				h.SetEdgeClient(tc.client)
			}
			_, err := h.ValidateAPIToken("tok-" + tc.name)
			require.Error(t, err)
			assert.True(t, errors.Is(err, coresvc.ErrCredentialCheckUnavailable), "got %v", err)
			assert.False(t, errors.Is(err, coresvc.ErrAppInactive))
		})
	}

	t.Run("rejection is a verdict, not unavailability", func(t *testing.T) {
		h := newStaleGraceService(t, 0)
		h.SetEdgeClient(&fakeEdgeClient{resp: &pb.TokenValidationResponse{Valid: false, ErrorMessage: "Invalid token"}})
		_, err := h.ValidateAPIToken("tok-bad")
		require.Error(t, err)
		assert.False(t, errors.Is(err, coresvc.ErrCredentialCheckUnavailable))
		assert.False(t, errors.Is(err, coresvc.ErrAppInactive))
	})

	t.Run("inactive App rejection is marked as such", func(t *testing.T) {
		h := newStaleGraceService(t, 0)
		h.SetEdgeClient(&fakeEdgeClient{resp: &pb.TokenValidationResponse{Valid: false, ErrorMessage: coresvc.AppInactiveMessage}})
		_, err := h.ValidateAPIToken("tok-inactive")
		require.Error(t, err)
		assert.True(t, errors.Is(err, coresvc.ErrAppInactive), "got %v", err)
		assert.False(t, errors.Is(err, coresvc.ErrCredentialCheckUnavailable))
	})

	t.Run("stale grace still serves before anything is classified", func(t *testing.T) {
		h := newStaleGraceService(t, time.Hour)
		seeded := seedExpiredEntry(h, "tok-grace", time.Minute)
		h.SetEdgeClient(&fakeEdgeClient{err: status.Error(codes.Unavailable, "down")})
		got, err := h.ValidateAPIToken("tok-grace")
		require.NoError(t, err)
		assert.Same(t, seeded, got)
	})
}

// The adapter wraps the service error for the proxy; the classification must
// survive the wrapping, since that is what the proxy tests with errors.Is.
func TestGatewayAdapter_KeepsValidationErrorClass(t *testing.T) {
	h := newStaleGraceService(t, 0)
	h.SetEdgeClient(&fakeEdgeClient{err: status.Error(codes.Unavailable, "down")})
	a := &GatewayServiceAdapter{gatewayService: h}

	_, err := a.GetCredentialBySecret("tok-adapter")
	require.Error(t, err)
	assert.True(t, errors.Is(err, coresvc.ErrCredentialCheckUnavailable), "got %v", err)
}
