package analytics

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func proxyLogVendors(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var vendors []string
	require.NoError(t, db.Model(&models.ProxyLog{}).Order("vendor").Pluck("vendor", &vendors).Error)
	return vendors
}

// One row the database refuses no longer loses its whole batch: the batch is
// written row by row and only that row is lost (jyx.88). On Postgres the row
// is an edge value over its column's size; SQLite does not enforce sizes, so
// there a primary key clash makes the batch fail and the retry reassigns it.
func TestProxyLogBatch_OneBadRowKeepsTheRest(t *testing.T) {
	captureDBs(t, func(t *testing.T, db *gorm.DB) {
		h := NewDatabaseHandler(context.Background(), db)
		t.Cleanup(h.Stop)

		existing := &models.ProxyLog{Vendor: "existing"}
		require.NoError(t, db.Create(existing).Error)

		bad := &models.ProxyLog{Vendor: "b-bad"}
		want := []string{"a-good", "c-good", "existing"}
		if db.Dialector.Name() == "postgres" {
			bad.RequestID = strings.Repeat("r", 100) // request_id is size:64
		} else {
			bad.ID = existing.ID // the batch fails on the duplicate key
			want = []string{"a-good", "b-bad", "c-good", "existing"}
		}
		h.RecordProxyLogsBatch(context.Background(), []*models.ProxyLog{
			{Vendor: "a-good"}, bad, {Vendor: "c-good"},
		})

		require.Eventually(t, func() bool { return len(proxyLogVendors(t, db)) == len(want) },
			5*time.Second, 20*time.Millisecond)
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, want, proxyLogVendors(t, db))
	})
}
