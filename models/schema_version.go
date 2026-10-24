package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// The schema version lets an instance that does not migrate (a headless
// control plane sharing a full Studio's database) check that the schema is
// one it understands.
//
// Rules for a change to the models:
//   - Every change to the schema (the goldens in testdata/schema) bumps
//     SchemaVersion; TestSchemaVersionMatchesGoldens and make schema-golden
//     fail until it is bumped.
//   - MinReaderSchemaVersion is raised, to the new SchemaVersion, only when
//     the change breaks code built for an older schema: a dropped or renamed
//     column or table, a changed column type, or a new NOT NULL column
//     without a default. Added tables, nullable or defaulted columns and
//     indexes leave it alone, so older readers keep working.
//   - A change to the format of stored data that older code misreads (values
//     encrypted at rest that it would use as written) also bumps
//     SchemaVersion and raises MinReaderSchemaVersion to it, even when the
//     goldens do not change: make schema-golden then rewrites only
//     testdata/schema/VERSION, with the same hash.
const (
	// 2: cluster_nodes.label and leader_eligible (nullable, additive).
	// 3: tyk_connections.host_key and its unique index (nullable, additive).
	// 4: endpoint_auth_plugins (new table, additive).
	// 5: proxy_logs and llm_chat_records on_behalf_of, acting_agent (nullable, additive).
	// 6: plugin_resource_types.default_access (nullable, additive).
	// 7: Tyk OSS Gateway connections: tyk_connections.kind (defaulted) and
	//    gateway_* columns, mcp_servers.key_access and gateway_coverage/partial
	//    (nullable), tyk_gateway_nodes and tyk_gateway_definitions (new tables).
	// 8: decision models: proxy_logs.request_kind and llm_chat_records
	//    request_kind (defaulted, indexed with time_stamp), decision_questions and
	//    decision_refusals (nullable, additive).
	// 9: native_resources (new table, additive): the native API extension
	//    ledger (Anthropic message batches).
	// 10: native API extensions follow-ups: llms resource_signing_*,
	//     vendor_workspace_id and price_multipliers, llm_chat_records and
	//     proxy_logs service_tier (nullable, additive).
	// 11: native_resources.screening_digest (nullable, additive).
	// 12: mcp_call_records (new table, additive): MCP analytics from Tyk Pump.
	// 13: tyk_connection_ingests and tyk_ingest_stats (new tables, additive):
	//     the analytics ingest token (hash only) and hourly counters.
	// 14: proxy_logs request_id (indexed) and client_request_id (nullable,
	//     additive): the gateway's request id (pkg/requestid) and the caller's.
	// 15: compliance_events.owner_visible (defaulted false, indexed with
	//     app_id and time_stamp; additive): guardrail events shared with App
	//     owners in the portal.
	// 16: agent_trace_* tables (agent tracing, new tables only).
	// 17: agent tracing settings: apps.tracing, llms/tools/datasources
	//     tracing_content_mode_max, llms.tracing_propagation_opt_out
	//     (nullable or defaulted, additive).
	// 18: agent_trace_traces/agent_trace_spans keyed by (app_id, id),
	//     agent_trace_span_links.app_id, App-scoped link/payload indexes.
	//     The tables are unreleased (added in 16); AutoMigrate cannot change
	//     a primary key, so databases created at 16 or 17 need the
	//     agent_trace tables dropped once.
	// 19: agent tracing read API: agent_trace_sessions.duration_ms and
	//     gateway_event_count (defaulted), indexes on sessions (started_at,
	//     id), spans (span_id) and traces (trace_id) (additive).
	// 20: proxy_logs.trace_span_id (indexed, nullable, additive); the
	//     proxy_logs.request_id agent tracing also uses is main's (14).
	// 21: agent_trace_traces.exported_at (nullable) and its index: the OTel
	//     exporter marks the turns it has sent (additive).
	// 22: partial indexes on agent_trace_traces over in-progress turns:
	//     idx_agent_trace_traces_open (app_id, session_id, turn_index) for
	//     gateway turns and idx_agent_trace_traces_idle (started_at)
	//     (additive).
	// 23: data format only, no schema change (aa5). LLM api_key and
	//     credential metadata are stored as $ENC/ when TYK_AI_SECRET_KEY is
	//     set; older code would use the ciphertext as the key, so the minimum
	//     reader is 23 and an older headless replica refuses the database.
	// 24: chats.render_images (defaulted false, additive): per-chat switch
	//     for rendering images in chat responses (2u2).
	SchemaVersion          = 24
	MinReaderSchemaVersion = 23
)

// studioSchemaRowID is the one row of studio_schema.
const studioSchemaRowID = 1

// StudioSchema records the schema version the database was last migrated
// to by a full Studio, and the oldest schema version whose code can still
// read it.
type StudioSchema struct {
	ID               uint `gorm:"primaryKey;autoIncrement:false"`
	Version          int  `gorm:"not null"`
	MinReaderVersion int  `gorm:"not null"`
	// WrittenBy is the Studio version that recorded it.
	WrittenBy string
	UpdatedAt time.Time
}

// TableName keeps the table name singular: it holds one row.
func (StudioSchema) TableName() string { return "studio_schema" }

var (
	// ErrSchemaMissing: no schema version is recorded. No Studio has
	// migrated the database, or the one that did predates schema versions.
	ErrSchemaMissing = errors.New("no Studio schema version is recorded in the database")
	// ErrSchemaTooOld: the database was migrated by an older Studio than
	// this build needs.
	ErrSchemaTooOld = errors.New("the database schema is older than this build needs")
	// ErrSchemaTooNew: a newer Studio migrated the database in a way this
	// build cannot read.
	ErrSchemaTooNew = errors.New("the database schema is newer than this build can read")
)

// RecordSchemaVersion records that the database now has this build's
// schema. Call it after every migration has run, under the migration lock.
// It never lowers the record: an older Studio started against a database a
// newer one migrated leaves the newer version in place (its own migrations
// only add), and logs that it did.
func RecordSchemaVersion(db *gorm.DB, writtenBy string) error {
	var current StudioSchema
	err := db.Take(&current, studioSchemaRowID).Error
	switch {
	case err == nil && current.Version > SchemaVersion:
		logger.Warnf("The database schema is version %d (recorded by Studio %s), newer than this build's %d; keeping the newer record", current.Version, current.WrittenBy, SchemaVersion)
		return nil
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		return fmt.Errorf("read schema version: %w", err)
	}

	row := StudioSchema{
		ID:               studioSchemaRowID,
		Version:          SchemaVersion,
		MinReaderVersion: MinReaderSchemaVersion,
		WrittenBy:        writtenBy,
		UpdatedAt:        time.Now(),
	}
	// Conditional in the write too, in case another instance recorded a
	// newer version since the read.
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"version", "min_reader_version", "written_by", "updated_at"}),
		Where: clause.Where{Exprs: []clause.Expression{
			clause.Lte{Column: clause.Column{Table: "studio_schema", Name: "version"}, Value: SchemaVersion},
		}},
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	return nil
}

// CheckSchemaVersion reports whether this build can use the database's
// schema without migrating it, and returns the record. It only reads: it
// creates nothing. The errors wrap ErrSchemaMissing, ErrSchemaTooOld or
// ErrSchemaTooNew.
func CheckSchemaVersion(ctx context.Context, db *gorm.DB) (StudioSchema, error) {
	db = db.WithContext(ctx)
	var row StudioSchema
	if err := ctx.Err(); err != nil {
		return row, err
	}
	if !db.Migrator().HasTable(&StudioSchema{}) {
		if err := ctx.Err(); err != nil {
			return row, err
		}
		return row, fmt.Errorf("%w: start a full Studio (schema version %d or later) against the database first", ErrSchemaMissing, SchemaVersion)
	}
	if err := db.Take(&row, studioSchemaRowID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, fmt.Errorf("%w: start a full Studio (schema version %d or later) against the database first", ErrSchemaMissing, SchemaVersion)
		}
		return row, fmt.Errorf("read schema version: %w", err)
	}
	if row.Version < SchemaVersion {
		return row, fmt.Errorf("%w: the database is at schema version %d (recorded by Studio %s), this build needs %d; upgrade the full Studio first",
			ErrSchemaTooOld, row.Version, row.WrittenBy, SchemaVersion)
	}
	if row.MinReaderVersion > SchemaVersion {
		return row, fmt.Errorf("%w: the database is at schema version %d (recorded by Studio %s), which needs schema version %d or later to read; this build has %d",
			ErrSchemaTooNew, row.Version, row.WrittenBy, row.MinReaderVersion, SchemaVersion)
	}
	return row, nil
}
