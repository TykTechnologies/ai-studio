package grpc

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Token validation for default-namespace edges keeps its namespace isolation
// in both editions (TestControlServer_NamespaceFiltering, which also covers
// these rows, is skipped in Enterprise builds): a global token validates from
// the default namespace in each spelling, a tenant token never does. A
// tenant edge is served in Community and refused in Enterprise.
func TestControlModeDefaultNamespaceTokenIsolation(t *testing.T) {
	s, db := newControlModeTestServer(t)
	ctx := context.Background()

	app := &database.App{Name: "Iso App", IsActive: true, Namespace: "tenant-a"}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Create(&database.APIToken{Token: "global-token-123", AppID: app.ID, IsActive: true}).Error)
	require.NoError(t, db.Create(&database.APIToken{Token: "tenant-a-token-123", AppID: app.ID, IsActive: true, Namespace: "tenant-a"}).Error)

	validate := func(token, ns string) (*pb.TokenValidationResponse, error) {
		return s.ValidateToken(ctx, &pb.TokenValidationRequest{Token: token, EdgeId: "iso-edge", EdgeNamespace: ns})
	}

	for _, ns := range []string{"", "global", "default"} {
		resp, err := validate("global-token-123", ns)
		require.NoError(t, err, ns)
		assert.True(t, resp.Valid, "global token from %q", ns)
		assert.Equal(t, uint32(app.ID), resp.AppId, ns)

		resp, err = validate("tenant-a-token-123", ns)
		require.NoError(t, err, ns)
		assert.False(t, resp.Valid, "tenant token must not validate from the default namespace %q", ns)
	}

	resp, err := validate("tenant-a-token-123", "tenant-a")
	if !controlModeDefaultNamespaceOnly {
		require.NoError(t, err)
		assert.True(t, resp.Valid, "Community serves tenant namespaces as before")
		return
	}
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Nil(t, resp)
}
