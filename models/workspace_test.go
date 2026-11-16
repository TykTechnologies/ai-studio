package models

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// stampedFixtures returns one unsaved row of every stamped model; n keeps
// names and slugs unique between calls.
func stampedFixtures(n int) []interface{} {
	name := fmt.Sprintf("ws-fixture-%d", n)
	return []interface{}{
		&LLM{Name: name},
		&App{Name: name},
		&Tool{Name: name},
		&Datasource{Name: name},
		&Filter{Name: name},
		&Embedder{Name: name},
		&ModelRouter{Name: name, Slug: name},
		&SemanticRouter{Name: name, Slug: name},
		&AgentConfig{Name: name, Slug: name},
		&Catalogue{Name: name},
		&DataCatalogue{Name: name},
		&ToolCatalogue{Name: name},
		&secrets.Secret{VarName: name},
		&FileStore{FileName: name},
		&MCPServer{Name: name},
		&Plugin{Name: name, Command: "file:///bin/true", HookType: "post_auth"},
		&WebhookTarget{ID: fmt.Sprintf("00000000-0000-0000-0000-%012d", n), Name: name, URL: "https://hooks.example", Status: "pending"},
	}
}

// createStampedFixtures writes the rows as an older build would: no hooks,
// workspace_id 0.
func createStampedFixtures(t *testing.T, db *gorm.DB, n int) []interface{} {
	t.Helper()
	rows := stampedFixtures(n)
	raw := db.Session(&gorm.Session{SkipHooks: true})
	for _, row := range rows {
		require.NoError(t, raw.Create(row).Error, "%T", row)
		require.NoError(t, raw.Model(row).Unscoped().UpdateColumn("workspace_id", 0).Error, "%T", row)
	}
	return rows
}

type stampedRow struct {
	WorkspaceID uint
	UpdatedAt   *time.Time
}

// stampedRows reads workspace_id and updated_at (where the table has one)
// of every row of every stamped table, soft-deleted rows included.
func stampedRows(t *testing.T, db *gorm.DB) map[string][]stampedRow {
	t.Helper()
	out := map[string][]stampedRow{}
	for _, m := range WorkspaceStampedModels() {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(m))
		cols := "workspace_id"
		if stmt.Schema.LookUpField("UpdatedAt") != nil {
			cols += ", updated_at"
		}
		var rows []stampedRow
		require.NoError(t, db.Table(stmt.Schema.Table).Select(cols).Scan(&rows).Error)
		out[stmt.Schema.Table] = rows
	}
	return out
}

func TestWorkspaceStampedModels_AreTheSeventeenTables(t *testing.T) {
	db := setupTestDB(t)
	var tables []string
	for _, m := range WorkspaceStampedModels() {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(m))
		tables = append(tables, stmt.Schema.Table)
		assert.True(t, db.Migrator().HasColumn(m, "workspace_id"), "%s has workspace_id", stmt.Schema.Table)
	}
	assert.ElementsMatch(t, []string{
		"llms", "apps", "tools", "datasources", "filters", "embedders",
		"model_routers", "semantic_routers", "agent_configs", "catalogues",
		"data_catalogues", "tool_catalogues", "secrets", "file_stores",
		"mcp_servers", "plugins", "webhook_targets",
	}, tables)
}

func TestEnsureDefaultWorkspace_CreatesExactlyOne(t *testing.T) {
	db := openPrivateMemoryDB(t, &gorm.Config{})
	require.NoError(t, InitModels(db))
	// A second start (and a direct second call) leave the one row alone.
	require.NoError(t, InitModels(db))
	again, err := EnsureDefaultWorkspace(db)
	require.NoError(t, err)

	var all []Workspace
	require.NoError(t, db.Find(&all).Error)
	require.Len(t, all, 1)
	assert.Equal(t, again.ID, all[0].ID)
	assert.True(t, all[0].IsDefault)
	assert.Equal(t, DefaultWorkspaceName, all[0].Name)

	def, err := GetDefaultWorkspace(db)
	require.NoError(t, err)
	assert.Equal(t, all[0].ID, def.ID)
}

func TestWorkspace_OneDefaultAndCaseInsensitiveNames(t *testing.T) {
	db := setupTestDB(t)
	_, err := EnsureDefaultWorkspace(db)
	require.NoError(t, err)

	require.NoError(t, db.Create(&Workspace{Name: "Team A"}).Error)
	assert.Error(t, db.Create(&Workspace{Name: "  team a "}).Error, "names are unique without regard to case")
	assert.Error(t, db.Create(&Workspace{Name: "Other", IsDefault: true}).Error, "only one Default workspace")
}

func TestBackfillWorkspaceIDs_StampsEveryRowWithoutTouchingUpdatedAt(t *testing.T) {
	db := setupTestDB(t)
	def, err := EnsureDefaultWorkspace(db)
	require.NoError(t, err)

	rows := createStampedFixtures(t, db, 1)
	// Soft-deleted rows are stamped too.
	require.NoError(t, db.Delete(rows[0]).Error) // the LLM
	require.NoError(t, db.Delete(rows[1]).Error) // the App
	// A fixed, past updated_at shows whether the backfill touched it.
	past := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, m := range WorkspaceStampedModels() {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(m))
		if stmt.Schema.LookUpField("UpdatedAt") != nil {
			require.NoError(t, db.Model(m).Unscoped().Where("1 = 1").UpdateColumn("updated_at", past).Error)
		}
	}

	before := stampedRows(t, db)
	for table, rs := range before {
		require.NotEmpty(t, rs, table)
		for _, r := range rs {
			require.Zero(t, r.WorkspaceID, table)
		}
	}

	require.NoError(t, BackfillWorkspaceIDs(db))
	first := stampedRows(t, db)
	for table, rs := range first {
		require.Len(t, rs, len(before[table]), table)
		for i, r := range rs {
			assert.Equal(t, def.ID, r.WorkspaceID, "%s row %d stamped", table, i)
			if before[table][i].UpdatedAt != nil {
				require.NotNil(t, r.UpdatedAt, table)
				assert.True(t, before[table][i].UpdatedAt.Equal(*r.UpdatedAt), "%s updated_at unchanged", table)
			}
		}
	}

	// Idempotent: a second run changes nothing.
	require.NoError(t, BackfillWorkspaceIDs(db))
	assert.Equal(t, first, stampedRows(t, db))
}

// A row an older build writes (workspace_id 0) after this build started is
// stamped on the next start, and a row already in another Workspace is
// never moved.
func TestBackfillWorkspaceIDs_NextBootStampsRowsFromAnOlderBuild(t *testing.T) {
	db := openPrivateMemoryDB(t, &gorm.Config{})
	require.NoError(t, InitModels(db))
	def, err := GetDefaultWorkspace(db)
	require.NoError(t, err)

	other := &Workspace{Name: "Elsewhere"}
	require.NoError(t, db.Create(other).Error)
	placed := &LLM{Name: "placed"}
	require.NoError(t, db.Create(placed).Error)
	require.NoError(t, db.Model(placed).UpdateColumn("workspace_id", other.ID).Error)

	createStampedFixtures(t, db, 2)

	require.NoError(t, InitModels(db)) // the next start
	for table, rs := range stampedRows(t, db) {
		for _, r := range rs {
			assert.NotZero(t, r.WorkspaceID, table)
		}
	}
	var got LLM
	require.NoError(t, db.First(&got, placed.ID).Error)
	assert.Equal(t, other.ID, got.WorkspaceID, "an object in another Workspace stays there")
	assert.NotEqual(t, def.ID, other.ID)
}

func TestEffectiveWorkspaceID(t *testing.T) {
	assert.Equal(t, uint(7), EffectiveWorkspaceID(0, 7), "0 is the Default workspace")
	assert.Equal(t, uint(3), EffectiveWorkspaceID(3, 7))
}

func TestKeepWorkspaceID(t *testing.T) {
	// Every stamped model: the original's workspace_id wins.
	for _, m := range WorkspaceStampedModels() {
		original := reflect.New(reflect.TypeOf(m).Elem())
		original.Elem().FieldByName("WorkspaceID").SetUint(4)
		modified := reflect.New(reflect.TypeOf(m).Elem())
		modified.Elem().FieldByName("WorkspaceID").SetUint(9)
		KeepWorkspaceID(original.Interface(), modified.Interface())
		assert.Equal(t, uint64(4), modified.Elem().FieldByName("WorkspaceID").Uint(), "%T", m)
	}
	zeroed := &Tool{}
	KeepWorkspaceID(&Tool{WorkspaceID: 4}, zeroed)
	assert.Equal(t, uint(4), zeroed.WorkspaceID, "a hook cannot zero it either")

	// Unstamped types, mismatched pairs and nil are left alone.
	u := &User{Name: "x"}
	KeepWorkspaceID(&User{}, u)
	assert.Equal(t, "x", u.Name)
	l := &LLM{WorkspaceID: 9}
	KeepWorkspaceID(&Tool{WorkspaceID: 4}, l)
	assert.Equal(t, uint(9), l.WorkspaceID)
	KeepWorkspaceID((*LLM)(nil), l)
	KeepWorkspaceID(&LLM{WorkspaceID: 4}, (*LLM)(nil))
	assert.Equal(t, uint(9), l.WorkspaceID)
}

// Workspace ids are never reused (AUTOINCREMENT on SQLite, a sequence on
// Postgres): a binding or stamp left pointing at a deleted Workspace must
// never come to mean a new one.
func TestWorkspaceIDsAreNotReused(t *testing.T) {
	db := setupTestDB(t)
	_, err := EnsureDefaultWorkspace(db)
	require.NoError(t, err)
	first := &Workspace{Name: "Short-lived"}
	require.NoError(t, db.Create(first).Error)
	require.NoError(t, db.Delete(first).Error)
	second := &Workspace{Name: "Next"}
	require.NoError(t, db.Create(second).Error)
	assert.Greater(t, second.ID, first.ID)
}
