package services

import (
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
		for _, row := range rows {
			if change, ok := classifyPendingChange(row, result.Since); ok {
				changes = append(changes, PendingChange{
					Type:   src.typ,
					ID:     row.ID,
					Name:   row.Name,
					Change: change.kind,
					At:     change.at,
				})
			}
		}
	}

	sort.SliceStable(changes, func(i, j int) bool {
		return changes[i].At.After(changes[j].At)
	})

	result.Total = len(changes)
	if len(changes) > maxPendingChanges {
		changes = changes[:maxPendingChanges]
	}
	if changes != nil {
		result.Changes = changes
	}
	return result, nil
}

// pendingRowsFor fetches the candidate rows of one source. Soft-deleted rows
// are included (Unscoped) only when there is a reference point to compare
// their deletion against.
func (s *SyncStatusService) pendingRowsFor(src pendingChangeSource, namespace string, since *time.Time) ([]pendingChangeRow, error) {
	query := s.db.Model(src.model).
		Select(src.nameExpr + " AS name, id, created_at, updated_at, deleted_at")

	if src.namespaced {
		if namespace == "" {
			query = query.Where("namespace = ''")
		} else {
			query = query.Where("(namespace = '' OR namespace = ?)", namespace)
		}
	}

	if since != nil {
		query = query.Unscoped().
			Where("(created_at > ? OR updated_at > ? OR deleted_at > ?)", *since, *since, *since)
	}

	var rows []pendingChangeRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
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
