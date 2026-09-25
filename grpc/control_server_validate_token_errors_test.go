package grpc

import (
	"context"
	"errors"
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// failQueriesOn makes every SELECT on table fail, as a hub database that has
// run out of disk or lost its connection does.
func failQueriesOn(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	name := "test:fail_" + table
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			_ = tx.AddError(errors.New("pq: could not extend file: No space left on device"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })
}

// A hub database fault is not a verdict on the token. It must reach the edge
// as an error, which the edge treats as "hub unavailable" (stale-grace
// fallback, cache kept), never as Valid:false, which makes the edge drop its
// cached entry and refuse the request with 401. The app lookup used to turn
// every error into "Associated app not found or inactive"; during a hub disk
// outage that sent edges into a 401 storm for valid credentials.
func TestControlServer_ValidateToken_DatabaseErrorsAreNotRejections(t *testing.T) {
	for _, table := range []string{"credentials", "apps"} {
		t.Run(table, func(t *testing.T) {
			server, db := setupTestServer(t, nil)
			token := "db-fault-token-" + table
			createTestCredentialAndApp(db, token)
			failQueriesOn(t, db, table)

			resp, err := server.ValidateToken(context.Background(), &pb.TokenValidationRequest{
				Token: token, EdgeId: "edge-001", EdgeNamespace: "test",
			})

			require.Error(t, err, "a database fault must be an error, got response %+v", resp)
			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.Internal, st.Code())
			assert.Nil(t, resp)
		})
	}
}
