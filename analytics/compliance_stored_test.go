package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hub hears of compliance events once they are stored, with their IDs
// (guardrail alerts name the compliance event they came from).
func TestComplianceEventsStoredHook(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_test_stored_hook=1"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.ComplianceEvent{}))

	got := make(chan []*models.ComplianceEvent, 1)
	SetComplianceEventsStored(func(events []*models.ComplianceEvent) { got <- events })
	t.Cleanup(func() { SetComplianceEventsStored(nil) })

	handler := NewDatabaseHandler(context.Background(), db)
	defer handler.Stop()
	handler.RecordComplianceEvents(context.Background(), []*models.ComplianceEvent{
		{AppID: 1, FilterName: "Wolf", EventType: "guardrail.prompt_injection", Metadata: `{"alert":"x1"}`, TimeStamp: time.Now()},
	})
	select {
	case events := <-got:
		require.Len(t, events, 1)
		assert.NotZero(t, events[0].ID, "stored, with its ID")
	case <-time.After(3 * time.Second):
		t.Fatal("the stored hook was not called")
	}
}
