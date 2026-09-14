package services

import (
	"database/sql/driver"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// Pending-changes preview for the push-configuration dialog: which
// gateway-visible objects were created, updated or deleted in a namespace
// since the last push (models.NamespaceSyncStatus.LastPushAt).

// maxPendingChanges caps the list returned; Total still reports the real
// count.
const maxPendingChanges = 200

// PendingChange is one object that changed since the last push.
type PendingChange struct {
	Type   string    `json:"type"`   // llm, app, filter, tool, datasource, plugin, model_router, model_price, oauth_client, access_token
	ID     uint      `json:"id"`     // object id
	Name   string    `json:"name"`   // display name at the time of the query
	Change string    `json:"change"` // created | updated | deleted
	At     time.Time `json:"at"`     // when that change happened
}

// PendingChanges is the response of GET /api/v1/sync/pending-changes.
type PendingChanges struct {
	Namespace string `json:"namespace"`
	// Since is the reference point (the namespace's last push); nil when
	// nothing has been pushed yet, in which case every object is "created".
	Since      *time.Time      `json:"since"`
	LastPushAt *time.Time      `json:"last_push_at"`
	Total      int             `json:"total"`
	Changes    []PendingChange `json:"changes"`
}

const (
	PendingChangeCreated = "created"
	PendingChangeUpdated = "updated"
	PendingChangeDeleted = "deleted"
)

// pendingChangeSource describes one table the configuration snapshot ships.
// nameExpr is the SQL expression for the display name; namespaced says
// whether the table has a namespace column (global tables ship to every
// namespace).
type pendingChangeSource struct {
	typ        string
	model      interface{}
	nameExpr   string
	namespaced bool
}

// pendingChangeSources mirrors what grpc.ControlServer.getConfigurationSnapshot
// puts on the wire, plus LLMs (apps reference them). Keep the two in step.
var pendingChangeSources = []pendingChangeSource{
	{typ: "llm", model: &models.LLM{}, nameExpr: "name", namespaced: true},
	{typ: "app", model: &models.App{}, nameExpr: "name", namespaced: true},
	{typ: "filter", model: &models.Filter{}, nameExpr: "name", namespaced: true},
	{typ: "tool", model: &models.Tool{}, nameExpr: "name", namespaced: true},
	{typ: "datasource", model: &models.Datasource{}, nameExpr: "name", namespaced: true},
	{typ: "plugin", model: &models.Plugin{}, nameExpr: "name", namespaced: true},
	{typ: "model_router", model: &models.ModelRouter{}, nameExpr: "name", namespaced: true},
	{typ: "model_price", model: &models.ModelPrice{}, nameExpr: "vendor || '/' || model_name", namespaced: false},
	{typ: "oauth_client", model: &models.OAuthClient{}, nameExpr: "client_name", namespaced: false},
	{typ: "access_token", model: &models.AccessToken{}, nameExpr: "client_id", namespaced: false},
}

// pendingChangeRow is one row of the UNION ALL over the source tables.
// The time columns come back through a compound select, which on SQLite
// drops the declared column type, so they are scanned by changeTime rather
// than time.Time. Total is COUNT(*) OVER () on the unlimited row set.
type pendingChangeRow struct {
	Typ       string
	ID        uint
	Name      string
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
// push. namespace "" and "global" both mean the global namespace, whose
// objects also ship to every other namespace; a named namespace sees its
// own objects plus the global ones.
func (s *SyncStatusService) GetPendingChanges(namespace string) (*PendingChanges, error) {
	if namespace == "global" {
		namespace = ""
	}

	result := &PendingChanges{Namespace: namespace, Changes: []PendingChange{}}

	var status models.NamespaceSyncStatus
	if err := status.GetByNamespace(s.db, namespace); err == nil {
		result.LastPushAt = status.LastPushAt
		result.Since = status.LastPushAt
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	rows, err := s.pendingRows(namespace, result.Since)
	if err != nil {
		return nil, err
	}

	var changes []PendingChange
	for _, row := range rows {
		result.Total = int(row.Total)
		if change, ok := classifyPendingChange(row, result.Since); ok {
			changes = append(changes, PendingChange{
				Type:   row.Typ,
				ID:     row.ID,
				Name:   row.Name,
				Change: change.kind,
				At:     change.at,
			})
		}
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
// round-trip rather than one or two per table.
//
// Soft-deleted rows are included only when there is a reference point to
// compare their deletion against. The time filter uses updated_at and
// deleted_at only: GORM stamps updated_at alongside created_at on insert,
// so a row created after the push is also updated after it, and a soft
// delete leaves updated_at alone.
func (s *SyncStatusService) pendingRows(namespace string, since *time.Time) ([]pendingChangeRow, error) {
	branches := make([]string, 0, len(pendingChangeSources))
	args := make([]interface{}, 0, 3*len(pendingChangeSources))
	for _, src := range pendingChangeSources {
		table, err := tableOf(s.db, src.model)
		if err != nil {
			return nil, err
		}

		// Bind order within a branch: type literal, then the WHERE markers.
		args = append(args, src.typ)
		where := make([]string, 0, 2)
		if src.namespaced {
			if namespace == "" {
				where = append(where, "namespace = ''")
			} else {
				where = append(where, "(namespace = '' OR namespace = ?)")
				args = append(args, namespace)
			}
		}
		if since == nil {
			where = append(where, "deleted_at IS NULL")
		} else {
			where = append(where, "(updated_at > ? OR deleted_at > ?)")
			args = append(args, *since, *since)
		}

		branches = append(branches, fmt.Sprintf(
			"SELECT ? AS typ, %s AS name, id, created_at, updated_at, deleted_at FROM %s WHERE %s",
			src.nameExpr, table, strings.Join(where, " AND ")))
	}

	sql := "SELECT c.*, COUNT(*) OVER () AS total FROM (" +
		strings.Join(branches, " UNION ALL ") +
		") AS c ORDER BY COALESCE(c.deleted_at, c.updated_at) DESC LIMIT ?"
	args = append(args, maxPendingChanges)

	var rows []pendingChangeRow
	if err := s.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
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
