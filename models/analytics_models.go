package models

// AnalyticsModels are the tables analytics.Migrate creates: the records the
// analytics recorder writes, including those from edge analytics pulses, so
// a control plane that does not migrate writes them too. They are listed
// here so the schema goldens (and so SchemaVersion) cover them. The agent
// tracing tables (AgentTraceModels) are migrated with them: the hub's pulse
// ingest writes the staging table. Edges never migrate these.
func AnalyticsModels() []interface{} {
	return append([]interface{}{
		&LLMChatRecord{},
		&LLMChatLogEntry{},
		&ToolCallRecord{},
		&ProxyLog{},
		&ComplianceEvent{},
		&MCPCallRecord{},
		// Which edge analytics batches were stored, written with their
		// records so one sent again is not stored twice.
		&EdgeAnalyticsBatch{},
	}, AgentTraceModels()...)
}
