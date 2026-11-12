package services

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// appState is what the credential sync wrote of an App.
type appState struct {
	Name          string
	IsActive      bool
	MonthlyBudget float64
	UpdatedAt     time.Time
	LLMs          int64
}

func readApp(t *testing.T, db *gorm.DB, id uint) appState {
	t.Helper()
	var a database.App
	require.NoError(t, db.Unscoped().First(&a, id).Error)
	var n int64
	require.NoError(t, db.Model(&database.AppLLM{}).Where("app_id = ?", id).Count(&n).Error)
	return appState{Name: a.Name, IsActive: a.IsActive, MonthlyBudget: a.MonthlyBudget, UpdatedAt: a.UpdatedAt, LLMs: n}
}

// appsPage is one full page of credentials, each with its own App.
func appsPage(e *syncedEdge, watermark uint64, apps ...*pb.AppConfig) pagesOf {
	page := &CredentialSyncPage{Full: true, Last: true, Watermark: watermark, HashVersion: "v1", Apps: apps}
	keys := []string{keyA, keyB, keyC}
	for i, app := range apps {
		page.Credentials = append(page.Credentials, SyncedCredentialUpdate{
			CredentialID: uint(10 + i), SecretHash: e.hash(keys[i]), AppID: uint(app.Id), Active: true, UpdatedAt: time.Now(),
		})
	}
	return pagesOf{page}
}

func withLLMs(app *pb.AppConfig, llms ...uint32) *pb.AppConfig {
	app.LlmIds = llms
	return app
}

// A sync stores an App only when it changed since the sync last stored it:
// a repeat full sync writes nothing for unchanged Apps, a change in any
// field of the App (one the edge does not store included) stores it again,
// and so does any other write of the App's row in between.
func TestCredentialSync_StoresOnlyChangedApps(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		e := newSyncedEdge(t, db, 24*time.Hour)
		one := withLLMs(pbApp(1, "app-one", true), 1, 2)
		two := withLLMs(pbApp(2, "app-two", true), 3)
		require.NoError(t, e.list.Sync(context.Background(), appsPage(e, 1, one, two)))
		first1, first2 := readApp(t, db, 1), readApp(t, db, 2)
		assert.Equal(t, appState{Name: "app-one", IsActive: true, UpdatedAt: first1.UpdatedAt, LLMs: 2}, first1)
		assert.EqualValues(t, 1, first2.LLMs)

		// Unchanged: not written again.
		require.NoError(t, e.list.Sync(context.Background(), appsPage(e, 2, proto.Clone(one).(*pb.AppConfig), proto.Clone(two).(*pb.AppConfig))))
		assert.Equal(t, first1, readApp(t, db, 1), "unchanged App not stored")
		assert.Equal(t, first2, readApp(t, db, 2))

		// A changed budget, and a field the edge does not keep: both stored.
		one.MonthlyBudget = 250
		two.AllowedIps = `["10.0.0.0/8"]`
		require.NoError(t, e.list.Sync(context.Background(), appsPage(e, 3, one, two)))
		assert.EqualValues(t, 250, readApp(t, db, 1).MonthlyBudget)
		assert.True(t, readApp(t, db, 1).UpdatedAt.After(first1.UpdatedAt))
		assert.True(t, readApp(t, db, 2).UpdatedAt.After(first2.UpdatedAt), "any field of the App counts")

		// Another writer changes the row (a key validated on demand brings
		// an older copy): the next sync stores the App again although the
		// control plane sent the same App as last time.
		require.NoError(t, e.h.storeAppFromPullOnMiss(pbApp(1, "renamed-elsewhere", true)))
		require.NoError(t, e.list.Sync(context.Background(), appsPage(e, 4, one, two)))
		got := readApp(t, db, 1)
		assert.Equal(t, "app-one", got.Name, "stored again after another write")
		assert.EqualValues(t, 2, got.LLMs)
		assert.EqualValues(t, 250, got.MonthlyBudget)

		// A soft-deleted App (the configuration snapshot retired it) is
		// stored again.
		require.NoError(t, db.Delete(&database.App{}, 2).Error)
		require.NoError(t, e.list.Sync(context.Background(), appsPage(e, 5, one, two)))
		var app database.App
		require.NoError(t, db.First(&app, 2).Error, "restored")
	})
}

// An inactive App arrives inactive, on insert too (a struct's false would
// be replaced by the column's default), and its keys are refused.
func TestCredentialSync_InactiveAppStaysInactive(t *testing.T) {
	forEachEdgeDB(t, func(t *testing.T, db *gorm.DB) {
		e := newSyncedEdge(t, db, 24*time.Hour)
		require.NoError(t, e.list.Sync(context.Background(), appsPage(e, 1, pbApp(1, "paused", false))))
		assert.False(t, readApp(t, db, 1).IsActive)
		_, err := e.h.ValidateAPIToken(keyA)
		require.Error(t, err)
	})
}
