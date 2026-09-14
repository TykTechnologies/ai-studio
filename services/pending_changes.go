package services

import (
	"fmt"
	"sort"
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

// pendingChangeRow is the projection scanned from each source table. The
// time columns are selected as plain columns (no aggregates) so SQLite
// hands them back as time values.
type pendingChangeRow struct {
	ID        uint
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
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

	var changes []PendingChange
	for _, src := range pendingChangeSources {
		rows, err := s.pendingRowsFor(src, namespace, result.Since)
		if err != nil {
			return nil, err
		}
		classified := 0
		for _, row := range rows {
			if change, ok := classifyPendingChange(row, result.Since); ok {
				classified++
				changes = append(changes, PendingChange{
					Type:   src.typ,
					ID:     row.ID,
					Name:   row.Name,
					Change: change.kind,
					At:     change.at,
				})
			}
		}
		// Each source is fetched newest-first and capped at maxPendingChanges,
		// so a full page means the table may hold more: count those rather
		// than load them, since only the newest maxPendingChanges overall are
		// returned anyway.
		if len(rows) < maxPendingChanges {
			result.Total += classified
		} else {
			n, err := s.pendingCountFor(src, namespace, result.Since)
			if err != nil {
				return nil, err
			}
			result.Total += int(n)
		}
	}

	sort.SliceStable(changes, func(i, j int) bool {
		return changes[i].At.After(changes[j].At)
	})

	if len(changes) > maxPendingChanges {
		changes = changes[:maxPendingChanges]
	}
	if changes != nil {
		result.Changes = changes
	}
	return result, nil
}

// pendingQueryFor builds the candidate-row query of one source. Soft-deleted
// rows are included (Unscoped) only when there is a reference point to
// compare their deletion against. The time filter uses updated_at and
// deleted_at only: GORM stamps updated_at alongside created_at on insert, so
// a row created after the push is also updated after it, and a soft delete
// leaves updated_at alone. Both columns are indexed
// (EnsurePendingChangeIndexes), which keeps the OR to two index scans.
func (s *SyncStatusService) pendingQueryFor(src pendingChangeSource, namespace string, since *time.Time) *gorm.DB {
	query := s.db.Model(src.model)

	if src.namespaced {
		if namespace == "" {
			query = query.Where("namespace = ''")
		} else {
			query = query.Where("(namespace = '' OR namespace = ?)", namespace)
		}
	}

	if since != nil {
		query = query.Unscoped().
			Where("(updated_at > ? OR deleted_at > ?)", *since, *since)
	}
	return query
}

// pendingRowsFor fetches the newest maxPendingChanges candidate rows of one
// source. The order key is the time of the change the row will be reported
// with (deletion time for a deleted row, otherwise its last update), so the
// per-source pages merge into the correct overall newest-first list.
func (s *SyncStatusService) pendingRowsFor(src pendingChangeSource, namespace string, since *time.Time) ([]pendingChangeRow, error) {
	var rows []pendingChangeRow
	err := s.pendingQueryFor(src, namespace, since).
		Select(src.nameExpr + " AS name, id, created_at, updated_at, deleted_at").
		Order("COALESCE(deleted_at, updated_at) DESC").
		Limit(maxPendingChanges).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// pendingCountFor counts the candidate rows of one source.
func (s *SyncStatusService) pendingCountFor(src pendingChangeSource, namespace string, since *time.Time) (int64, error) {
	var n int64
	err := s.pendingQueryFor(src, namespace, since).Count(&n).Error
	return n, err
}

// EnsurePendingChangeIndexes creates the updated_at index each source table
// needs for the since-last-push filter (gorm.Model indexes deleted_at
// already). Idempotent; run once at start-up after the tables exist.
func EnsurePendingChangeIndexes(db *gorm.DB) error {
	if db == nil {
		return nil // tests build a Service without a database
	}
	for _, src := range pendingChangeSources {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(src.model); err != nil {
			return fmt.Errorf("pending changes: parse %s model: %w", src.typ, err)
		}
		table := stmt.Schema.Table
		sql := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_updated_at ON %s (updated_at)", table, table)
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("pending changes: index %s.updated_at: %w", table, err)
		}
	}
	return nil
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
		return pendingChangeKind{kind: PendingChangeCreated, at: row.CreatedAt}, true
	}
	switch {
	case row.DeletedAt.Valid && row.DeletedAt.Time.After(*since):
		return pendingChangeKind{kind: PendingChangeDeleted, at: row.DeletedAt.Time}, true
	case row.DeletedAt.Valid:
		// Deleted before the push: already gone from the edge's view.
		return pendingChangeKind{}, false
	case row.CreatedAt.After(*since):
		return pendingChangeKind{kind: PendingChangeCreated, at: row.CreatedAt}, true
	case row.UpdatedAt.After(*since):
		return pendingChangeKind{kind: PendingChangeUpdated, at: row.UpdatedAt}, true
	}
	return pendingChangeKind{}, false
}
