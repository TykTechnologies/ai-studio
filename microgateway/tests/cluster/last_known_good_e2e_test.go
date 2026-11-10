package cluster

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/microgateway/internal/api/handlers"
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	edgegrpc "github.com/TykTechnologies/midsommar/microgateway/internal/grpc"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// These tests restart an edge while no control plane can be reached
// (EDGE_START_FROM_LAST_KNOWN_GOOD). See features/ActiveActive.md, Phase 3.

// lkgEdge is an edge process with a database that outlives it: the edge
// client, the sync service and the snapshot store wired as cmd/microgateway
// wires them.
type lkgEdge struct {
	db     *gorm.DB
	client *edgegrpc.SimpleEdgeClient
	// lkg is the stored snapshot the edge started from; nil when it
	// connected to a control plane.
	lkg *services.LastKnownGood
}

type lkgEdgeOptions struct {
	lastKnownGood bool
	// startupTimeout is EDGE_STARTUP_CONTROL_TIMEOUT.
	startupTimeout time.Duration
}

// runEdge starts an edge process on the database at path and returns it, or
// the error that makes it exit.
func runEdge(t *testing.T, path, id string, opts lkgEdgeOptions, fwds ...*forwarder) (*lkgEdge, error) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	t.Cleanup(func() { database.Close(db) })
	// What a new process starts with: nothing in memory.
	scripting.SetPromptTemplateSource(nil)
	scripting.SetExceptionSource(nil)

	cfg := &config.Config{
		Security: config.SecurityConfig{EncryptionKey: encryptionKey},
		HubSpoke: config.HubSpokeConfig{
			Mode:                   "edge",
			EdgeID:                 id,
			ReconnectInterval:      100 * time.Millisecond,
			HeartbeatInterval:      300 * time.Millisecond,
			SyncTimeout:            5 * time.Second,
			ClientToken:            authToken,
			AllowInsecure:          true,
			StartFromLastKnownGood: opts.lastKnownGood,
			StartupControlTimeout:  opts.startupTimeout,
			LastKnownGoodMaxAge:    72 * time.Hour,
		},
	}
	for _, f := range fwds {
		cfg.HubSpoke.ControlEndpoints = append(cfg.HubSpoke.ControlEndpoints, f.addr())
	}
	require.NoError(t, services.PersistAppliedSnapshots(db, encryptionKey))
	client := edgegrpc.NewSimpleEdgeClient(cfg, "e2e", "hash", "now")
	client.SetControlContactHook(func(at time.Time) { _ = services.RecordControlContact(db, at) })
	syncService := services.NewEdgeSyncService(db, "")

	lkg, err := client.StartWithLastKnownGood(db)
	if err != nil {
		_ = client.Stop()
		return nil, err
	}
	t.Cleanup(func() { _ = client.Stop() })

	client.SetOnConfigChange(func(snap *pb.ConfigurationSnapshot) {
		if err := syncService.SyncConfiguration(snap); err != nil {
			t.Logf("edge %s: sync failed: %v", id, err)
		}
	})
	client.SetAppliedConfigSource(syncService.ReportedConfig)
	// The configuration the client already has: from the registration, or
	// the stored snapshot.
	if initial := client.GetCurrentConfiguration(); initial != nil {
		require.NoError(t, syncService.SyncConfiguration(initial))
	}
	client.SetReloadHandler(services.NewEdgeReloadHandler(client, syncService, db, id, func(resp *pb.ConfigurationReloadResponse) {
		_ = client.SendReloadStatus(resp)
	}, nil))
	if lkg != nil {
		client.StartReconnecting()
	}
	return &lkgEdge{db: db, client: client, lkg: lkg}, nil
}

func (e *lkgEdge) stop(t *testing.T) {
	t.Helper()
	require.NoError(t, e.client.Stop())
	require.NoError(t, database.Close(e.db))
}

func (e *lkgEdge) count(t *testing.T, model interface{}, where string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	q := e.db.Model(model)
	if where != "" {
		q = q.Where(where, args...)
	}
	require.NoError(t, q.Count(&n).Error)
	return n
}

// ready calls the edge's /ready handler.
func (e *lkgEdge) ready(t *testing.T) (int, map[string]interface{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)
	handlers.ReadinessCheck(&services.ServiceContainer{
		DB: e.db,
		ControlLink: func() (string, bool) {
			link := e.client.ControlLink()
			return link.Endpoint, link.Connected
		},
		ConfigSource: e.client.ConfigSource,
	})(c)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w.Code, body
}

// hubConfiguration gives the hub an LLM, an App that uses it, a guardrail
// filter, an approved prompt template and an approved guardrail exception:
// tables on the edge, and state an edge holds only in memory.
func hubConfiguration(t *testing.T, db *gorm.DB, key string) (app *models.App) {
	t.Helper()
	llm := &models.LLM{Name: "lkg-llm", Vendor: models.OPENAI, Active: true, APIEndpoint: "https://api.openai.com/v1", DefaultModel: "gpt-4o-mini"}
	require.NoError(t, db.Create(llm).Error)
	cred := &models.Credential{KeyID: "lkg-key-id", Secret: key, Active: true}
	require.NoError(t, db.Create(cred).Error)
	app = &models.App{Name: "lkg-app", IsActive: true, CredentialID: cred.ID, LLMs: []models.LLM{*llm}}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Model(&models.App{}).Where("id = ?", app.ID).Update("guardrail_capability", "read_only").Error)

	filter := &models.Filter{Name: "lkg-guardrail", Kind: models.FilterKindGuardrail, Config: models.JSONMap{"provider": "http", "on_detect": "block"}}
	require.NoError(t, db.Create(filter).Error)

	now := time.Now()
	tpl := &models.AppPromptTemplate{
		AppID: app.ID, Name: "review", Version: 1, Status: templates.StatusApproved, ExpiresAfterDays: 30, ApprovedAt: &now,
		Parts: []templates.Part{
			{Const: "You are reviewing a pull request. Follow the review rules below.\n<diff>\n"},
			{Slot: "diff"},
			{Const: "\n</diff>\nReport only real defects, one per line, with file and line.\n"},
		},
	}
	tpl.SetPlacement(templates.Placement{Role: templates.RoleUser}.Normalize())
	require.NoError(t, tpl.Create(db))

	require.NoError(t, db.Create(&models.GuardrailException{
		AppID: app.ID, FilterID: filter.ID, Detector: "prompt_attack", Channel: "text_user", Action: "log",
		Status: models.GuardrailExceptionApproved, Justification: "e2e", ExpiresAfterDays: 30, ApprovedAt: &now,
	}).Error)
	return app
}

func edgeRow(t *testing.T, db *gorm.DB, edgeID string) models.EdgeInstance {
	t.Helper()
	var row models.EdgeInstance
	require.NoError(t, db.Where("edge_id = ?", edgeID).First(&row).Error)
	return row
}

// Every control plane is cut and the edge restarts. With
// EDGE_START_FROM_LAST_KNOWN_GOOD it comes up from the snapshot it stored:
// its tables and what it holds only in memory are loaded, /ready is 200 and
// says so, and a key it must ask a control plane about cannot be checked.
// When a control plane returns, the edge takes the live configuration.
func TestE2E_LastKnownGood_RestartWithEveryControlPlaneCut(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		t.Cleanup(func() {
			scripting.SetPromptTemplateSource(nil)
			scripting.SetExceptionSource(nil)
		})
		a := startReplica(t, sdb, "node-a")
		b := startReplica(t, sdb, "node-b")
		const key = "5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f"
		app := hubConfiguration(t, a.db, key)
		fwdA, fwdB := newForwarder(t, a.addr), newForwarder(t, b.addr)
		path := filepath.Join(t.TempDir(), "edge-1.db")

		// First run: connected. The edge applies and stores the snapshot.
		first, err := runEdge(t, path, "edge-1", lkgEdgeOptions{lastKnownGood: true, startupTimeout: 5 * time.Second}, fwdA, fwdB)
		require.NoError(t, err)
		require.Nil(t, first.lkg, "a control plane answered")
		waitOwner(t, a.db, "edge-1", "node-a")
		require.EqualValues(t, 1, first.count(t, &database.LLM{}, "name = ?", "lkg-llm"))
		require.Equal(t, 1, services.InstalledPromptTemplates(app.ID))
		require.Equal(t, 1, services.InstalledGuardrailExceptions(app.ID))
		code, body := first.ready(t)
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, "control", body["config_source"])
		stored, err := services.LoadAppliedSnapshot(first.db, encryptionKey)
		require.NoError(t, err)
		reportedBefore := stored.Checksum
		// The hub has seen the edge's heartbeat with that checksum.
		require.Eventually(t, func() bool { return edgeRow(t, a.db, "edge-1").LoadedChecksum == reportedBefore },
			10*time.Second, 25*time.Millisecond)
		first.stop(t)

		// Both sites are cut, and the configuration changes on the hub
		// while the edge is away.
		fwdA.fail()
		fwdB.fail()
		addLLM(t, b.db, "added-during-outage")

		// The edge restarts with no control plane.
		started := time.Now()
		second, err := runEdge(t, path, "edge-1", lkgEdgeOptions{lastKnownGood: true, startupTimeout: 500 * time.Millisecond}, fwdA, fwdB)
		require.NoError(t, err, "an edge with a stored snapshot starts without a control plane")
		require.NotNil(t, second.lkg)
		t.Logf("edge up from last-known-good %v after start", time.Since(started).Round(time.Millisecond))

		code, body = second.ready(t)
		assert.Equal(t, http.StatusOK, code, "the edge is ready: %v", body)
		assert.Equal(t, "last_known_good", body["config_source"])
		assert.Equal(t, false, body["control_connected"])

		// The stored configuration is loaded: tables and memory.
		assert.EqualValues(t, 1, second.count(t, &database.LLM{}, "name = ?", "lkg-llm"), "the LLM")
		assert.EqualValues(t, 1, second.count(t, &database.Filter{}, "name = ?", "lkg-guardrail"), "the filter")
		assert.EqualValues(t, 1, second.count(t, &database.App{}, "id = ?", app.ID), "the App")
		assert.EqualValues(t, 1, second.count(t, &database.AppLLM{}, "app_id = ?", app.ID), "the App's LLM grant")
		assert.Equal(t, 1, services.InstalledPromptTemplates(app.ID), "the prompt template, held only in memory")
		assert.Equal(t, 1, services.InstalledGuardrailExceptions(app.ID), "the guardrail exception, held only in memory")
		assert.Zero(t, second.count(t, &database.LLM{}, "name = ?", "added-during-outage"), "what changed since is not there")
		sum, _, ok := services.NewEdgeSyncService(second.db, "").ReportedConfig()
		require.True(t, ok)
		assert.Equal(t, reportedBefore, sum, "the edge reports the checksum of the configuration it restored")

		// An App key has to be checked with a control plane (the token
		// cache is empty after a restart), and there is none: the gateway
		// answers 503 credential_check_unavailable for this error.
		_, err = second.client.ValidateTokenOnDemand(key)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not connected to control instance")

		// A control plane returns. The edge reconnects on its own and takes
		// the live configuration.
		fwdB.setUp()
		waitOwner(t, b.db, "edge-1", "node-b")
		require.Eventually(t, func() bool {
			return second.count(t, &database.LLM{}, "name = ?", "added-during-outage") == 1
		}, 20*time.Second, 25*time.Millisecond, "the live configuration was not applied on reconnect")
		code, body = second.ready(t)
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, "control", body["config_source"])
		assert.Equal(t, true, body["control_connected"])

		live, err := services.LoadAppliedSnapshot(second.db, encryptionKey)
		require.NoError(t, err)
		assert.NotEqual(t, reportedBefore, live.Checksum, "the live snapshot replaced the stored one")
		require.Eventually(t, func() bool {
			row := edgeRow(t, b.db, "edge-1")
			return row.SyncStatus == models.EdgeSyncStatusInSync && row.LoadedChecksum == live.Checksum
		}, 20*time.Second, 25*time.Millisecond, "the hub shows the edge in sync with the live configuration: %+v", edgeRow(t, b.db, "edge-1"))

		resp, err := second.client.ValidateTokenOnDemand(key)
		require.NoError(t, err)
		assert.True(t, resp.Valid, "keys validate again through the control plane")
	})
}

// The cases in which an edge may not start from last-known-good exit with
// the reason, as an edge without the option does.
func TestE2E_LastKnownGood_Refusals(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		addLLM(t, a.db, "lkg-llm")
		fwd := newForwarder(t, a.addr)
		dir := t.TempDir()
		path := filepath.Join(dir, "edge-1.db")

		first, err := runEdge(t, path, "edge-1", lkgEdgeOptions{lastKnownGood: true, startupTimeout: 5 * time.Second}, fwd)
		require.NoError(t, err)
		waitOwner(t, a.db, "edge-1", "node-a")
		first.stop(t)
		fwd.fail()

		// Without the option: as today.
		_, err = runEdge(t, path, "edge-1", lkgEdgeOptions{}, fwd)
		require.Error(t, err)
		var refusal *services.LastKnownGoodRefusal
		assert.False(t, errors.As(err, &refusal), "without the option the stored snapshot is not considered: %v", err)

		// A database that did not survive the restart (a rescheduled pod
		// without a persistent volume).
		_, err = runEdge(t, filepath.Join(dir, "new.db"), "edge-1", lkgEdgeOptions{lastKnownGood: true}, fwd)
		require.Error(t, err)
		require.True(t, errors.As(err, &refusal), "%v", err)
		assert.Contains(t, refusal.Reason, "no configuration snapshot is stored")

		// With its database, the same edge starts.
		again, err := runEdge(t, path, "edge-1", lkgEdgeOptions{lastKnownGood: true}, fwd)
		require.NoError(t, err)
		require.NotNil(t, again.lkg)
		assert.EqualValues(t, 1, again.count(t, &database.LLM{}, "name = ?", "lkg-llm"))
	})
}
