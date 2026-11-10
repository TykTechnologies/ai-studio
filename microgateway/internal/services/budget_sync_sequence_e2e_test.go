//go:build !enterprise

// As pricing_regression_e2e_test.go: the Studio side is linked in the CE
// build only.

package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	studiogrpc "github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hub's budget.sync sequence against a real edge handler. The edge
// holds a sequence from a hub before 2.3, the wall clock of a leader running
// a minute ahead of this one: it applies the upgraded hub's first sync. A
// second hub replica (a new leader, whatever its clock says) continues the
// sequence, and the edge applies its first sync too. Edge and hub both on
// SQLite, and both on Postgres (MGW_TEST_POSTGRES_DSN, DATABASE_URL).
func TestBudgetSyncSequence_UpgradedHubAndLeaderChangePostgres(t *testing.T) {
	for _, kind := range []string{"sqlite", "postgres"} {
		t.Run(kind, func(t *testing.T) {
			hub := hubDB(t, kind)
			var edge *gorm.DB
			if kind == "sqlite" {
				edge = setupBudgetSyncHandlerTestDB(t)
			} else {
				edge = openPostgresSchema(t)
			}
			t.Setenv("BUDGET_SYNC_INTERVAL", "1h") // one sync per Start

			owner := models.User{Email: "sequence@example.com", Name: "Sequence"}
			require.NoError(t, hub.Create(&owner).Error)
			cred := models.Credential{KeyID: "sequence-key", Secret: "sequence-secret"}
			require.NoError(t, hub.Create(&cred).Error)
			app := models.App{Name: "App 1", UserID: owner.ID, CredentialID: cred.ID}
			require.NoError(t, hub.Create(&app).Error)
			now := time.Now()
			periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			spend := func(cost float64) {
				t.Helper()
				require.NoError(t, hub.Create(&models.LLMChatRecord{AppID: app.ID, LLMID: 1, Cost: cost, TimeStamp: periodStart.Add(time.Hour)}).Error)
			}
			spend(250000) // $25

			// The edge knows the App and last applied a 2.2 leader's sync.
			edgeApp := database.App{Name: "App 1", IsActive: true}
			require.NoError(t, edge.Create(&edgeApp).Error)
			require.Equal(t, app.ID, edgeApp.ID)
			wallClock := uint64(now.Add(time.Minute).UnixNano())
			require.NoError(t, edge.Create(&database.SyncState{Topic: BudgetSyncTopic, SequenceNumber: wallClock, LastSyncAt: now}).Error)
			handler := NewBudgetSyncHandler(edge)
			handler.ledger = NewBudgetLedger(edge)
			require.Equal(t, wallClock, handler.GetLastSequenceNumber())

			bus := eventbridge.NewBus()
			bus.Subscribe(BudgetSyncTopic, handler.HandleBudgetSync)
			edgeSpend := func() float64 {
				var usage database.BudgetUsage
				if err := edge.Where("app_id = ?", app.ID).First(&usage).Error; err != nil {
					return 0
				}
				return usage.TotalCost
			}
			syncFrom := func(leader string, want float64) uint64 {
				t.Helper()
				before := handler.GetLastSequenceNumber()
				svc := studiogrpc.NewBudgetSyncService(hub, bus)
				svc.Start()
				defer svc.Stop()
				require.Eventually(t, func() bool { return handler.GetLastSequenceNumber() > before },
					10*time.Second, 20*time.Millisecond, "the edge applies the first sync from %s", leader)
				assert.InDelta(t, want, edgeSpend(), 1, "spend from %s", leader)
				return handler.GetLastSequenceNumber()
			}

			first := syncFrom("the upgraded hub", 250000)
			spend(100000)
			second := syncFrom("the next leader", 350000)
			assert.Equal(t, first+1, second, "the next leader continues the sequence")

			// The sequence the edge stored reads back as it was written.
			assert.Equal(t, second, NewBudgetSyncHandler(edge).GetLastSequenceNumber())
		})
	}
}
