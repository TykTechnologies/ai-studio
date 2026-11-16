package services

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Pending-changes preview for the push-configuration dialog: which
// gateway-visible objects were created, updated or deleted in a namespace
// since the last push (models.NamespaceSyncStatus.LastPushAt).

// maxPendingChanges caps the list returned; Total still reports the real
// count.
const maxPendingChanges = 200

// PendingChange is one object that changed since the last push.
type PendingChange struct {
	Type string `json:"type"` // llm, app, filter, tool, datasource, plugin, model_router, semantic_router, model_price, oauth_client, access_token
	// ID and Name are left out (Hidden set) for an object the caller cannot
	// see: an App or router in a Workspace their read does not cover.
	ID     uint      `json:"id,omitempty"`   // object id
	Name   string    `json:"name,omitempty"` // display name at the time of the query
	Change string    `json:"change"`         // created | updated | deleted
	At     time.Time `json:"at"`             // when that change happened
	// WorkspaceID is the Workspace that owns the object (a stored 0 reads
	// as the Default workspace); nil for types with no owning Workspace
	// (plugin, model_price, oauth_client, access_token).
	WorkspaceID   *uint  `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	// Hidden marks a change whose object the caller cannot see; its type,
	// kind, time and Workspace id are still reported.
	Hidden bool `json:"hidden,omitempty"`
}

// PendingWorkspaceTotal counts the changes owned by one Workspace.
type PendingWorkspaceTotal struct {
	WorkspaceID uint   `json:"workspace_id"`
	Name        string `json:"name"`
	Total       int    `json:"total"`
}

// PendingChanges is the response of GET /api/v1/sync/pending-changes.
type PendingChanges struct {
	// Namespace is the canonical spelling (models.CanonicalNamespace).
	Namespace string `json:"namespace"`
	// Since is the reference point: the namespace's last push, or -- when
	// no push was ever recorded but an edge is in sync with the namespace
	// checksum -- that edge's ack. Nil when there is neither, in which case
	// every object is "created". Baseline says which.
	Since      *time.Time      `json:"since"`
	LastPushAt *time.Time      `json:"last_push_at"`
	Baseline   string          `json:"baseline"` // push | edge_ack | none
	Total      int             `json:"total"`
	Changes    []PendingChange `json:"changes"`
	// ByWorkspace counts every pending change (not only the listed ones)
	// by owning Workspace; UnattributedTotal counts those with none.
	ByWorkspace       []PendingWorkspaceTotal `json:"by_workspace"`
	UnattributedTotal int                     `json:"unattributed_total"`
	// Set when asked about one Workspace (?workspace_id=): OutsideTotal
	// counts the changes foreign to it (PendingChangeIsForeign) and Stained
	// reports whether there are any, i.e. whether a push limited to that
	// Workspace would carry changes from outside it.
	WorkspaceID  *uint `json:"workspace_id,omitempty"`
	OutsideTotal *int  `json:"outside_total,omitempty"`
	Stained      *bool `json:"stained,omitempty"`
}

// Baseline values: what Since was taken from.
const (
	PendingBaselinePush    = "push"     // the namespace's recorded last push
	PendingBaselineEdgeAck = "edge_ack" // an in-sync edge's ack; no push recorded (pre-upgrade database)
	PendingBaselineNone    = "none"     // nothing to measure from
)

const (
	PendingChangeCreated = "created"
	PendingChangeUpdated = "updated"
	PendingChangeDeleted = "deleted"
)

// pendingChangeSource describes one table the configuration snapshot ships.
// nameExpr is the SQL expression for the display name; namespaced says
// whether the table has a namespace column (global tables ship to every
// namespace).
//
// stamped says whether the type's owning Workspace is reported
// (workspace_id, 0 read as the Default workspace). staining marks the
// unstamped types whose changes are foreign to any Workspace-limited push
// (global configuration with no enforced owner); oauth_client and
// access_token are neither: runtime artifacts that never stain.
type pendingChangeSource struct {
	typ        string
	model      interface{}
	nameExpr   string
	namespaced bool
	stamped    bool
	staining   bool
}

// pendingChangeSources mirrors what grpc.ControlServer.getConfigurationSnapshot
// puts on the wire, plus LLMs (apps reference them). Keep the two in step.
var pendingChangeSources = []pendingChangeSource{
	{typ: "llm", model: &models.LLM{}, nameExpr: "name", namespaced: true, stamped: true},
	{typ: "app", model: &models.App{}, nameExpr: "name", namespaced: true, stamped: true},
	{typ: "filter", model: &models.Filter{}, nameExpr: "name", namespaced: true, stamped: true},
	{typ: "tool", model: &models.Tool{}, nameExpr: "name", namespaced: true, stamped: true},
	{typ: "datasource", model: &models.Datasource{}, nameExpr: "name", namespaced: true, stamped: true},
	// Plugins carry the stamp, but the resource is global (.36): a plugin
	// change stains every Workspace-limited push.
	{typ: "plugin", model: &models.Plugin{}, nameExpr: "name", namespaced: true, staining: true},
	{typ: "model_router", model: &models.ModelRouter{}, nameExpr: "name", namespaced: true, stamped: true},
	{typ: "semantic_router", model: &models.SemanticRouter{}, nameExpr: "name", namespaced: true, stamped: true},
	// Embedders travel flattened inside the datasources and routers that use
	// them; an edit changes those without touching their rows.
	{typ: "embedder", model: &models.Embedder{}, nameExpr: "name", namespaced: false, stamped: true},
	{typ: "model_price", model: &models.ModelPrice{}, nameExpr: "vendor || '/' || model_name", namespaced: false, staining: true},
	{typ: "oauth_client", model: &models.OAuthClient{}, nameExpr: "client_name", namespaced: false},
	{typ: "access_token", model: &models.AccessToken{}, nameExpr: "client_id", namespaced: false},
}

// pendingSourceByType finds a source by its type name.
func pendingSourceByType(typ string) (pendingChangeSource, bool) {
	for _, src := range pendingChangeSources {
		if src.typ == typ {
			return src, true
		}
	}
	return pendingChangeSource{}, false
}

// PendingChangeIsForeign reports whether a pending change is foreign to a
// push limited to workspaceID: a change on a stamped type owned by another
// Workspace, or on plugin or model_price (global configuration). OAuth
// clients and access tokens never are.
func PendingChangeIsForeign(typ string, changeWorkspaceID *uint, workspaceID uint) bool {
	src, ok := pendingSourceByType(typ)
	if !ok {
		return false
	}
	if src.staining {
		return true
	}
	return src.stamped && changeWorkspaceID != nil && *changeWorkspaceID != workspaceID
}

// pendingChangeRow is one row of the UNION ALL over the source tables.
// The time columns come back through a compound select, which on SQLite
// drops the declared column type, so they are scanned by changeTime rather
// than time.Time. Total is COUNT(*) OVER () on the unlimited row set.
type pendingChangeRow struct {
	Typ  string
	ID   uint
	Name string
	// Ws is the owning Workspace (0 read as Default); NULL for unstamped
	// types.
	Ws        *uint
	CreatedAt changeTime
	UpdatedAt changeTime
	DeletedAt changeTime
	Total     int64
}

// changeTime scans a time column whether the driver hands back a time.Time
// (Postgres, SQLite with a declared type) or the raw TEXT SQLite stores
// (compound selects lose the declared type). Valid is false for NULL.
type changeTime struct {
	Time  time.Time
	Valid bool
}

// sqliteTimeFormats are the layouts go-sqlite3 writes and accepts.
var sqliteTimeFormats = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
	time.RFC3339Nano,
}

func (t *changeTime) Scan(v interface{}) error {
	switch x := v.(type) {
	case nil:
		*t = changeTime{}
		return nil
	case time.Time:
		*t = changeTime{Time: x, Valid: true}
		return nil
	case []byte:
		return t.parse(string(x))
	case string:
		return t.parse(x)
	}
	return fmt.Errorf("pending changes: cannot scan %T into a time", v)
}

// Value satisfies driver.Valuer so GORM treats the struct as a scalar
// column rather than a relation; the type is never written back.
func (t changeTime) Value() (driver.Value, error) {
	if !t.Valid {
		return nil, nil
	}
	return t.Time, nil
}

func (t *changeTime) parse(s string) error {
	s = strings.TrimSuffix(s, "Z")
	for _, layout := range sqliteTimeFormats {
		if parsed, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			*t = changeTime{Time: parsed, Valid: true}
			return nil
		}
	}
	return fmt.Errorf("pending changes: unrecognised time %q", s)
}

// GetPendingChanges lists what changed in the namespace since its last
// push. "", "global" and "default" all mean the default namespace
// (models.CanonicalNamespace), which ships the global objects plus those
// filed under "default"; a named namespace sees its own objects plus the
// global ones. This mirrors grpc.ControlServer.getConfigurationSnapshot.
//
// When no push was recorded (a database from before last_push_at existed,
// or edges that only ever got their configuration when they registered)
// the latest time an edge confirmed it was in sync is the reference point
// (Baseline edge_ack) rather than reporting everything as never pushed.
// Offline edges count: the configuration as of their ack reached them.
func (s *SyncStatusService) GetPendingChanges(namespace string) (*PendingChanges, error) {
	return s.GetPendingChangesFor(namespace, nil)
}

// GetPendingChangesFor is GetPendingChanges, also reporting, when
// workspaceID is set, how many changes are foreign to that Workspace
// (OutsideTotal, Stained): what a push limited to it would carry from
// outside it. The caller checks that the Workspace exists.
func (s *SyncStatusService) GetPendingChangesFor(namespace string, workspaceID *uint) (*PendingChanges, error) {
	namespace = models.CanonicalNamespace(namespace)

	result := &PendingChanges{Namespace: namespace, Baseline: PendingBaselineNone, Changes: []PendingChange{}}

	var status models.NamespaceSyncStatus
	if err := status.GetByNamespace(s.db, namespace); err == nil {
		result.LastPushAt = status.LastPushAt
		result.Since = status.LastPushAt
		if status.LastPushAt != nil {
			result.Baseline = PendingBaselinePush
		}
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if result.Since == nil {
		acked, err := (&models.EdgeInstance{}).LatestSyncAck(s.db, namespace)
		if err != nil {
			return nil, err
		}
		if acked != nil && acked.LastSyncAck != nil {
			since := *acked.LastSyncAck
			result.Since = &since
			result.Baseline = PendingBaselineEdgeAck
		}
	}

	defaultWorkspaceID, err := s.defaultWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.pendingRows(namespace, result.Since, defaultWorkspaceID)
	if err != nil {
		return nil, err
	}
	totals, err := s.pendingTotals(namespace, result.Since, defaultWorkspaceID)
	if err != nil {
		return nil, err
	}
	names, err := s.workspaceNames(rows, totals)
	if err != nil {
		return nil, err
	}

	var changes []PendingChange
	for _, row := range rows {
		result.Total = int(row.Total)
		if change, ok := classifyPendingChange(row, result.Since); ok {
			pc := PendingChange{
				Type:        row.Typ,
				ID:          row.ID,
				Name:        row.Name,
				Change:      change.kind,
				At:          change.at,
				WorkspaceID: row.Ws,
			}
			if row.Ws != nil {
				pc.WorkspaceName = names[*row.Ws]
			}
			changes = append(changes, pc)
		}
	}

	byWorkspace := map[uint]int{}
	outside := 0
	for _, t := range totals {
		if t.Ws == nil {
			result.UnattributedTotal += int(t.N)
		} else {
			byWorkspace[*t.Ws] += int(t.N)
		}
		if workspaceID != nil && PendingChangeIsForeign(t.Typ, t.Ws, *workspaceID) {
			outside += int(t.N)
		}
	}
	result.ByWorkspace = make([]PendingWorkspaceTotal, 0, len(byWorkspace))
	for id, n := range byWorkspace {
		result.ByWorkspace = append(result.ByWorkspace, PendingWorkspaceTotal{WorkspaceID: id, Name: names[id], Total: n})
	}
	sort.Slice(result.ByWorkspace, func(i, j int) bool { return result.ByWorkspace[i].WorkspaceID < result.ByWorkspace[j].WorkspaceID })
	if workspaceID != nil {
		id, stained := *workspaceID, outside > 0
		result.WorkspaceID, result.OutsideTotal, result.Stained = &id, &outside, &stained
	}

	// The database orders by the column the change time is derived from;
	// re-sort on the derived value so ties and created-then-updated rows
	// land in a stable newest-first order.
	sort.SliceStable(changes, func(i, j int) bool {
		return changes[i].At.After(changes[j].At)
	})

	if changes != nil {
		result.Changes = changes
	}
	return result, nil
}

// pendingRows runs one query over every source table: a UNION ALL of the
// per-table candidate selects, ordered newest-first by the time the change
// will be reported with (deletion time for a deleted row, otherwise its
// last update) and capped at maxPendingChanges, with the uncapped count
// carried on every row by a window function. Each branch filters on its
// own indexed columns before the union, so the cap and the count cost one
// round-trip rather than one or two per table (pendingUnion).
func (s *SyncStatusService) pendingRows(namespace string, since *time.Time, defaultWorkspaceID uint) ([]pendingChangeRow, error) {
	union, args, err := s.pendingUnion(namespace, since, defaultWorkspaceID, false)
	if err != nil {
		return nil, err
	}
	sql := "SELECT c.*, COUNT(*) OVER () AS total FROM (" + union +
		") AS c ORDER BY COALESCE(c.deleted_at, c.updated_at) DESC LIMIT ?"
	args = append(args, maxPendingChanges)

	var rows []pendingChangeRow
	if err := s.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// pendingTotal is one (type, Workspace) count of pending changes.
type pendingTotal struct {
	Typ string
	Ws  *uint
	N   int64
}

// pendingTotals counts every pending change (beyond the listed cap) by type
// and owning Workspace. The union is the listing's, narrowed to the rows
// classifyPendingChange reports (a row deleted at or before the reference
// point is no change), so the counts agree with what is listed.
func (s *SyncStatusService) pendingTotals(namespace string, since *time.Time, defaultWorkspaceID uint) ([]pendingTotal, error) {
	union, args, err := s.pendingUnion(namespace, since, defaultWorkspaceID, true)
	if err != nil {
		return nil, err
	}
	var totals []pendingTotal
	err = s.db.Raw("SELECT c.typ, c.ws, COUNT(*) AS n FROM ("+union+") AS c GROUP BY c.typ, c.ws", args...).Scan(&totals).Error
	return totals, err
}

// pendingUnion builds one query over every source table: a UNION ALL of the
// per-table candidate selects. Each branch filters on its own indexed
// columns. ws is the owning Workspace for stamped types, a stored 0 read as
// the Default workspace (rows an older replica wrote during a rolling
// upgrade), and NULL otherwise. exact also drops the rows deleted at or
// before since: classifyPendingChange reports no change for them, so the
// totals (pendingTotals, exact) must leave them out to agree with the
// listing (pendingRows, not exact, whose Total keeps its old meaning).
//
// Soft-deleted rows are included only when there is a reference point to
// compare their deletion against. The time filter uses updated_at and
// deleted_at only: GORM stamps updated_at alongside created_at on insert,
// so a row created after the push is also updated after it, and a soft
// delete leaves updated_at alone.
func (s *SyncStatusService) pendingUnion(namespace string, since *time.Time, defaultWorkspaceID uint, exact bool) (string, []interface{}, error) {
	branches := make([]string, 0, len(pendingChangeSources))
	args := make([]interface{}, 0, 4*len(pendingChangeSources))
	for _, src := range pendingChangeSources {
		table, err := tableOf(s.db, src.model)
		if err != nil {
			return "", nil, err
		}

		// Bind order within a branch: type literal, the Workspace default,
		// then the WHERE markers.
		args = append(args, src.typ)
		ws := "CAST(NULL AS INTEGER)"
		if src.stamped {
			ws = "COALESCE(NULLIF(workspace_id, 0), ?)"
			args = append(args, defaultWorkspaceID)
		}
		where := make([]string, 0, 3)
		if src.namespaced {
			// Same filter as the configuration snapshot: global objects
			// plus the namespace's own (the default namespace's own are
			// those filed under "default").
			where = append(where, "(namespace = '' OR namespace = ?)")
			args = append(args, models.CanonicalNamespace(namespace))
		}
		if since == nil {
			where = append(where, "deleted_at IS NULL")
		} else {
			where = append(where, "(updated_at > ? OR deleted_at > ?)")
			args = append(args, *since, *since)
			if exact {
				where = append(where, "(deleted_at IS NULL OR deleted_at > ?)")
				args = append(args, *since)
			}
		}

		branches = append(branches, fmt.Sprintf(
			"SELECT ? AS typ, %s AS name, id, %s AS ws, created_at, updated_at, deleted_at FROM %s WHERE %s",
			src.nameExpr, ws, table, strings.Join(where, " AND ")))
	}
	return strings.Join(branches, " UNION ALL "), args, nil
}

// WorkspaceExists reports whether a Workspace with the id exists.
func (s *SyncStatusService) WorkspaceExists(id uint) (bool, error) {
	var n int64
	err := s.db.Model(&models.Workspace{}).Where("id = ?", id).Count(&n).Error
	return n > 0, err
}

// defaultWorkspaceID is the Default workspace's id (0 when it does not
// exist yet, before the start-up bootstrap). A lookup failure is an error:
// guessing would misattribute every change written with workspace_id 0.
func (s *SyncStatusService) defaultWorkspaceID() (uint, error) {
	w, err := models.GetDefaultWorkspace(s.db)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("pending changes: load the default workspace: %w", err)
	}
	return w.ID, nil
}

// workspaceNames names the Workspaces the rows and totals mention.
func (s *SyncStatusService) workspaceNames(rows []pendingChangeRow, totals []pendingTotal) (map[uint]string, error) {
	seen := map[uint]bool{}
	var ids []uint
	add := func(ws *uint) {
		if ws != nil && !seen[*ws] {
			seen[*ws] = true
			ids = append(ids, *ws)
		}
	}
	for _, r := range rows {
		add(r.Ws)
	}
	for _, t := range totals {
		add(t.Ws)
	}
	names := map[uint]string{}
	if len(ids) == 0 {
		return names, nil
	}
	var workspaces []models.Workspace
	if err := s.db.Select("id", "name").Where("id IN ?", ids).Find(&workspaces).Error; err != nil {
		return nil, err
	}
	for _, w := range workspaces {
		names[w.ID] = w.Name
	}
	return names, nil
}

// tableOf resolves the table a model maps to.
func tableOf(db *gorm.DB, model interface{}) (string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return "", fmt.Errorf("pending changes: parse %T: %w", model, err)
	}
	return stmt.Schema.Table, nil
}

// EnsurePendingChangeIndexes creates the index each source table needs for
// the since-last-push filter: (namespace, updated_at) where the query also
// narrows by namespace, plain (updated_at) on global tables (gorm.Model
// indexes deleted_at already). Idempotent; run once at start-up after the
// tables exist.
func EnsurePendingChangeIndexes(db *gorm.DB) error {
	if db == nil {
		return nil // tests build a Service without a database
	}
	for _, src := range pendingChangeSources {
		table, err := tableOf(db, src.model)
		if err != nil {
			return err
		}
		name, columns := pendingChangeIndex(table, src.namespaced)
		sql := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s %s", name, table, columns)
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("pending changes: index %s: %w", name, err)
		}
	}
	return nil
}

// pendingChangeIndex names the index EnsurePendingChangeIndexes creates for
// a table and lists its columns.
func pendingChangeIndex(table string, namespaced bool) (name, columns string) {
	if namespaced {
		return fmt.Sprintf("idx_%s_ns_updated_at", table), "(namespace, updated_at)"
	}
	return fmt.Sprintf("idx_%s_updated_at", table), "(updated_at)"
}

type pendingChangeKind struct {
	kind string
	at   time.Time
}

// classifyPendingChange decides what happened to a row relative to since.
// Deleted wins over updated wins over created; an object both created and
// deleted after the push is reported as deleted (the edge never saw it, but
// the administrator may want to know it is gone).
func classifyPendingChange(row pendingChangeRow, since *time.Time) (pendingChangeKind, bool) {
	if since == nil {
		if row.DeletedAt.Valid {
			return pendingChangeKind{}, false
		}
		return pendingChangeKind{kind: PendingChangeCreated, at: row.CreatedAt.Time}, true
	}
	switch {
	case row.DeletedAt.Valid && row.DeletedAt.Time.After(*since):
		return pendingChangeKind{kind: PendingChangeDeleted, at: row.DeletedAt.Time}, true
	case row.DeletedAt.Valid:
		// Deleted before the push: already gone from the edge's view.
		return pendingChangeKind{}, false
	case row.CreatedAt.Time.After(*since):
		return pendingChangeKind{kind: PendingChangeCreated, at: row.CreatedAt.Time}, true
	case row.UpdatedAt.Time.After(*since):
		return pendingChangeKind{kind: PendingChangeUpdated, at: row.UpdatedAt.Time}, true
	}
	return pendingChangeKind{}, false
}
