package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/marketplace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A marketplace source on an internal address is not fetched when it is
// synced (by the schedule or by hand), whoever stored it and however: the
// sync fails with the one fixed error, which is also what is stored as the
// source's sync error and shown to anyone who may list sources.
func TestMarketplaceSyncRefusesAnInternalIndex(t *testing.T) {
	ms, db := setupMarketplaceTest(t)
	require.NoError(t, db.AutoMigrate(&models.MarketplaceIndex{}))

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("apiVersion: v1\nplugins: {}\n"))
	}))
	t.Cleanup(srv.Close)

	for _, source := range []string{
		srv.URL + "/index.yaml",
		"http://169.254.169.254/latest/meta-data/",
	} {
		idx := &models.MarketplaceIndex{SourceURL: source, IsActive: true, SyncStatus: "pending"}
		require.NoError(t, db.Create(idx).Error)

		for _, force := range []bool{false, true} {
			err := ms.SyncIndex(context.Background(), idx, force)
			require.ErrorIs(t, err, marketplace.ErrFetchFailed, source)
		}

		var stored models.MarketplaceIndex
		require.NoError(t, db.First(&stored, idx.ID).Error)
		assert.Equal(t, "error", stored.SyncStatus)
		assert.Equal(t, marketplace.ErrFetchFailed.Error(), stored.SyncError, "the stored error does not say what answered")
	}
	assert.Zero(t, hits.Load(), "no request may reach the internal server")

	// The documented setting still lets an internal index sync.
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	idx := &models.MarketplaceIndex{SourceURL: srv.URL + "/allowed.yaml", IsActive: true, SyncStatus: "pending"}
	require.NoError(t, db.Create(idx).Error)
	err := ms.SyncIndex(context.Background(), idx, false)
	require.NotErrorIs(t, err, marketplace.ErrFetchFailed)
	assert.Positive(t, hits.Load())
}
