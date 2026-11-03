package services

import (
	"errors"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/studiodb"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type driftFixture struct {
	svc    *Service
	db     *gorm.DB
	app    *models.App
	llm    *models.LLM
	tool   *models.Tool
	tool2  *models.Tool
	ds     *models.Datasource
	admin  *models.User
	writer *models.User
}

// newDriftFixture builds an App attested read_only with one tool; guarded
// attaches a guardrail filter to its LLM.
func newDriftFixture(t *testing.T, guarded bool) *driftFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, studiodb.Migrate(db))
	svc := NewService(db)
	svc.NotificationService = NewTestNotificationService(db)

	admin := &models.User{Email: "admin@test.com", Name: "Admin", IsAdmin: true, NotificationsEnabled: true, EmailVerified: true}
	require.NoError(t, admin.Create(db))
	writer := &models.User{Email: "dev@test.com", Name: "Dev", NotificationsEnabled: true, EmailVerified: true}
	require.NoError(t, writer.Create(db))

	llm := &models.LLM{Name: "gpt", Vendor: "openai", Active: true}
	require.NoError(t, db.Create(llm).Error)
	if guarded {
		filter := &models.Filter{Name: "Wolf", Kind: models.FilterKindGuardrail}
		require.NoError(t, db.Create(filter).Error)
		require.NoError(t, db.Model(llm).Association("Filters").Append(filter))
	}
	tool := &models.Tool{Name: "Repo", ToolType: models.ToolTypeREST}
	require.NoError(t, db.Create(tool).Error)
	tool2 := &models.Tool{Name: "Issues", ToolType: models.ToolTypeREST}
	require.NoError(t, db.Create(tool2).Error)
	ds := &models.Datasource{Name: "Docs"}
	require.NoError(t, db.Create(ds).Error)

	app := &models.App{Name: "Visor", UserID: writer.ID}
	require.NoError(t, app.Create(db))
	require.NoError(t, db.Model(app).Association("LLMs").Append(llm))
	require.NoError(t, db.Model(app).Association("Tools").Append(tool))
	_, err = svc.SetAppGuardrailCapability(app.ID, "read_only", admin.ID)
	require.NoError(t, err)

	return &driftFixture{svc: svc, db: db, app: app, llm: llm, tool: tool, tool2: tool2, ds: ds, admin: admin, writer: writer}
}

func (f *driftFixture) stored(t *testing.T) *models.App {
	t.Helper()
	a, err := f.svc.GetAppByID(f.app.ID)
	require.NoError(t, err)
	return a
}

func (f *driftFixture) resetNotifications(t *testing.T) []models.Notification {
	t.Helper()
	var n []models.Notification
	require.NoError(t, f.db.Where("notification_id LIKE ?", "guardrail_capability_reset_%").Find(&n).Error)
	return n
}

func TestCapabilityDrift_NonAdminToolChangeResetsVisibly(t *testing.T) {
	f := newDriftFixture(t, true)
	app, err := f.svc.AddToolToApp(f.app.ID, f.tool2.ID, WithCapabilityActor(f.writer.ID, false))
	require.NoError(t, err)
	assert.True(t, app.GuardrailCapabilityReset)
	assert.Equal(t, "", app.GuardrailCapability)

	got := f.stored(t)
	assert.Equal(t, "", got.GuardrailCapability)
	require.NotNil(t, got.GuardrailCapabilityResetBy)
	assert.Equal(t, f.writer.ID, *got.GuardrailCapabilityResetBy)
	assert.NotNil(t, got.GuardrailCapabilityResetAt)

	n := f.resetNotifications(t)
	require.Len(t, n, 1, "the full administrators are asked to re-attest")
	assert.Equal(t, f.admin.ID, n[0].UserID)
	assert.Equal(t, "/admin/apps/"+strconv.FormatUint(uint64(f.app.ID), 10), n[0].Link)

	// Attesting again clears the record of the reset.
	_, err = f.svc.SetAppGuardrailCapability(f.app.ID, "read_tools", f.admin.ID)
	require.NoError(t, err)
	got = f.stored(t)
	assert.Equal(t, "read_tools", got.GuardrailCapability)
	assert.Nil(t, got.GuardrailCapabilityResetBy)
	assert.Nil(t, got.GuardrailCapabilityResetAt)
}

// Security's probe (.41): AddToolToApp with no actor named (a plugin, any
// caller that does not say) must not keep the attestation.
func TestCapabilityDrift_UnnamedCallerResets(t *testing.T) {
	f := newDriftFixture(t, true)
	app, err := f.svc.AddToolToApp(f.app.ID, f.tool2.ID)
	require.NoError(t, err)
	assert.True(t, app.GuardrailCapabilityReset)
	got := f.stored(t)
	assert.Equal(t, "", got.GuardrailCapability)
	assert.Nil(t, got.GuardrailCapabilityResetBy, "no user is known")
	assert.NotNil(t, got.GuardrailCapabilityResetAt)
}

func TestCapabilityDrift_FullAdminKeeps(t *testing.T) {
	f := newDriftFixture(t, true)
	app, err := f.svc.AddToolToApp(f.app.ID, f.tool2.ID, WithCapabilityActor(f.admin.ID, true))
	require.NoError(t, err)
	assert.False(t, app.GuardrailCapabilityReset)
	assert.Equal(t, "read_only", f.stored(t).GuardrailCapability)
	assert.Empty(t, f.resetNotifications(t))
}

func TestCapabilityDrift_QuietWithoutGuardrails(t *testing.T) {
	f := newDriftFixture(t, false)
	app, err := f.svc.AddToolToApp(f.app.ID, f.tool2.ID, WithCapabilityActor(f.writer.ID, false))
	require.NoError(t, err)
	assert.False(t, app.GuardrailCapabilityReset, "no notice")
	got := f.stored(t)
	assert.Equal(t, "", got.GuardrailCapability, "still cleared")
	assert.Nil(t, got.GuardrailCapabilityResetAt)
	assert.Nil(t, got.GuardrailCapabilityResetBy)
	assert.Empty(t, f.resetNotifications(t), "no notification")
}

func TestCapabilityDrift_EveryResourcePath(t *testing.T) {
	writer := func(f *driftFixture) AppOption { return WithCapabilityActor(f.writer.ID, false) }
	cases := map[string]func(t *testing.T, f *driftFixture) error{
		"RemoveToolFromApp": func(t *testing.T, f *driftFixture) error {
			reset, err := f.svc.RemoveToolFromAppReportingReset(f.app.ID, f.tool.ID, writer(f))
			assert.True(t, reset)
			return err
		},
		"AddDatasourceToApp": func(t *testing.T, f *driftFixture) error {
			return f.svc.AddDatasourceToApp(f.app.ID, f.ds.ID, writer(f))
		},
		"UpdateApp tools": func(t *testing.T, f *driftFixture) error {
			app, err := f.svc.UpdateApp(f.app.ID, "Visor", "", f.writer.ID, nil, []uint{f.llm.ID}, []uint{f.tool.ID, f.tool2.ID}, nil, nil, nil, writer(f))
			if err == nil {
				assert.True(t, app.GuardrailCapabilityReset)
				assert.Equal(t, "", app.GuardrailCapability)
			}
			return err
		},
		"UpdateApp datasources (management gRPC: no actor)": func(t *testing.T, f *driftFixture) error {
			_, err := f.svc.UpdateApp(f.app.ID, "Visor", "", f.writer.ID, []uint{f.ds.ID}, []uint{f.llm.ID}, []uint{f.tool.ID}, nil, nil, nil)
			return err
		},
		"SetAppMCPServers": func(t *testing.T, f *driftFixture) error {
			srv := &models.MCPServer{Name: "Jira", Slug: "jira"}
			require.NoError(t, f.db.Create(srv).Error)
			app, err := f.svc.SetAppMCPServers(f.app.ID, []uint{srv.ID}, writer(f))
			if err == nil {
				assert.True(t, app.GuardrailCapabilityReset)
			}
			return err
		},
		"SetAppPluginResources": func(t *testing.T, f *driftFixture) error {
			prt := &models.PluginResourceType{PluginID: 1, Slug: "kb", Name: "Knowledge base"}
			require.NoError(t, f.db.Create(prt).Error)
			return f.svc.SetAppPluginResources(f.app.ID, prt.ID, []string{"kb-1"}, writer(f))
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := newDriftFixture(t, true)
			require.NoError(t, change(t, f))
			got := f.stored(t)
			assert.Equal(t, "", got.GuardrailCapability)
			assert.NotNil(t, got.GuardrailCapabilityResetAt)
		})
	}
}

func TestCapabilityDrift_NoResourceChangeKeeps(t *testing.T) {
	f := newDriftFixture(t, true)
	// Same tools, LLMs and no datasources: only the name changes.
	app, err := f.svc.UpdateApp(f.app.ID, "Visor 2", "new description", f.writer.ID, nil, []uint{f.llm.ID}, []uint{f.tool.ID}, nil, nil, nil, WithCapabilityActor(f.writer.ID, false))
	require.NoError(t, err)
	assert.False(t, app.GuardrailCapabilityReset)
	assert.Equal(t, "read_only", f.stored(t).GuardrailCapability)
}

// .42: a save of the whole App (UpdateApp's) must not write back a
// capability loaded before an administrator changed it.
func TestAppUpdate_DoesNotWriteBackStaleCapability(t *testing.T) {
	f := newDriftFixture(t, false)
	stale := f.stored(t) // loaded at read_only, as UpdateApp loads it
	_, err := f.svc.SetAppGuardrailCapability(f.app.ID, "acting", f.admin.ID)
	require.NoError(t, err)

	stale.Description = "x"
	require.NoError(t, stale.Update(f.db))

	got := f.stored(t)
	assert.Equal(t, "x", got.Description)
	assert.Equal(t, "acting", got.GuardrailCapability)

	// PatchAppMetadata saves the App too.
	_, err = f.svc.PatchAppMetadata(f.app.ID, "k", `"v"`, false)
	require.NoError(t, err)
	assert.Equal(t, "acting", f.stored(t).GuardrailCapability)
}

// .44: plugin resources are something Studio cannot classify.
func TestSuggestAppCapability_PluginResourcesAreActing(t *testing.T) {
	f := newDriftFixture(t, false)
	require.NoError(t, f.db.Model(f.app).Association("Tools").Clear())
	prt := &models.PluginResourceType{PluginID: 7, Slug: "kb", Name: "Knowledge base"}
	require.NoError(t, f.db.Create(prt).Error)
	require.NoError(t, f.svc.SetAppPluginResources(f.app.ID, prt.ID, []string{"kb-1"}, WithCapabilityActor(f.admin.ID, true)))

	sug, err := f.svc.SuggestAppCapabilityFor(f.stored(t))
	require.NoError(t, err)
	assert.Equal(t, "acting", string(sug.Suggestion))
	require.Len(t, sug.Reasons, 1)
	assert.Contains(t, sug.Reasons[0], `"Knowledge base" of plugin 7`)
}

// .48: when the reset cannot be written the call fails, rather than leave
// the attestation over the changed resources and answer as if all went well.
func TestCapabilityDrift_ResetFailureFailsClosed(t *testing.T) {
	f := newDriftFixture(t, true)
	attempts := 0
	require.NoError(t, f.db.Callback().Update().Before("gorm:update").Register("test:fail_capability_reset", func(tx *gorm.DB) {
		if m, ok := tx.Statement.Dest.(map[string]any); ok {
			if v, ok := m["guardrail_capability"]; ok && v == "" {
				attempts++
				tx.AddError(errors.New("injected reset failure"))
			}
		}
	}))
	_, err := f.svc.AddToolToApp(f.app.ID, f.tool2.ID, WithCapabilityActor(f.writer.ID, false))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrGuardrailCapabilityReset)
	assert.Equal(t, 2, attempts, "retried once")

	require.NoError(t, f.db.Callback().Update().Remove("test:fail_capability_reset"))
	// A failure on the first try only is retried and succeeds.
	failOnce := true
	require.NoError(t, f.db.Callback().Update().Before("gorm:update").Register("test:fail_once", func(tx *gorm.DB) {
		if m, ok := tx.Statement.Dest.(map[string]any); ok {
			if v, ok := m["guardrail_capability"]; ok && v == "" && failOnce {
				failOnce = false
				tx.AddError(errors.New("injected reset failure"))
			}
		}
	}))
	app, err := f.svc.RemoveToolFromAppReportingReset(f.app.ID, f.tool.ID, WithCapabilityActor(f.writer.ID, false))
	require.NoError(t, err)
	assert.True(t, app)
	assert.Equal(t, "", f.stored(t).GuardrailCapability)
}
