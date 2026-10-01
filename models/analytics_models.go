package models

// AnalyticsModels are the tables analytics.Migrate creates: the records the
// analytics recorder writes, including those from edge analytics pulses, so
// a control plane that does not migrate writes them too. They are listed
// here so the schema goldens (and so SchemaVersion) cover them.
func AnalyticsModels() []interface{} {
	return []interface{}{
		&LLMChatRecord{},
		&LLMChatLogEntry{},
		&ToolCallRecord{},
		&ProxyLog{},
		&ComplianceEvent{},
	}
}
