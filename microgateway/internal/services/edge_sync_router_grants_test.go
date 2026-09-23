package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// An App's Model Router grants arrive in AppConfig.model_router_ids and must
// land in app_model_routers, so the gateway can see them on the App, and be
// replaced (not accumulated) by the next snapshot.
func TestEdgeSync_AppModelRouterGrants(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	namespace := ""
	snapshot := createFullSnapshot(namespace)
	snapshot.ModelRouters = []*pb.ModelRouterConfig{{
		Id: 7, Name: "Prod", Slug: "prod", IsActive: true, Namespace: namespace,
		Pools: []*pb.ModelPoolConfig{{
			Id: 1, Name: "all", ModelPattern: "*", SelectionAlgorithm: "round_robin",
			Vendors: []*pb.PoolVendorConfig{{Id: 1, LlmId: 1, LlmSlug: "test-llm", Weight: 1, IsActive: true}},
		}},
		CreatedAt: timestamppb.Now(), UpdatedAt: timestamppb.Now(),
	}}
	snapshot.Apps[0].ModelRouterIds = []uint32{7}

	sync := NewEdgeSyncService(db, namespace)
	require.NoError(t, sync.SyncConfiguration(snapshot))

	var app database.App
	require.NoError(t, db.Preload("ModelRouters").First(&app, snapshot.Apps[0].Id).Error)
	require.Len(t, app.ModelRouters, 1)
	assert.Equal(t, uint(7), app.ModelRouters[0].ID)
	assert.Equal(t, "prod", app.ModelRouters[0].Slug)

	// The next snapshot revokes the grant.
	snapshot.Apps[0].ModelRouterIds = nil
	require.NoError(t, sync.SyncConfiguration(snapshot))
	var count int64
	require.NoError(t, db.Model(&database.AppModelRouter{}).Count(&count).Error)
	assert.Zero(t, count)
}
