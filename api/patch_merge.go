package api

import (
	"encoding/json"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// PATCH on datasources, tools and embedders bound onto inputs whose
// attributes are plain values, so an omitted attribute arrived as its zero
// value and was written as such: a PATCH of one field blanked the name,
// owner, privacy score and credentials (96e, as dg5 was for Apps and
// mergeLLMPatch for LLMs). These fill every attribute the body left out
// from the stored record; a present key, empty or null included, still
// replaces. Attributes that are already pointers (omitted = unchanged) are
// left to their handlers.

// mergeDatasourcePatch keeps the datasource's stored value for every
// attribute the body left out. The embed_* fields already keep the current
// embedder's values when empty (pickDatasourceEmbedder), and active is
// handled by updateDatasource.
func mergeDatasourcePatch(input *DatasourceInput, existing *models.Datasource, present map[string]json.RawMessage) {
	attrs := &input.Data.Attributes
	has := func(k string) bool { _, ok := present[k]; return ok }
	if !has("name") {
		attrs.Name = existing.Name
	}
	if !has("short_description") {
		attrs.ShortDescription = existing.ShortDescription
	}
	if !has("long_description") {
		attrs.LongDescription = existing.LongDescription
	}
	if !has("icon") {
		attrs.Icon = existing.Icon
	}
	if !has("url") {
		attrs.Url = existing.Url
	}
	if !has("privacy_score") {
		attrs.PrivacyScore = existing.PrivacyScore
	}
	if !has("user_id") {
		attrs.UserID = existing.UserID
	}
	if !has("tags") {
		attrs.Tags = make([]string, 0, len(existing.Tags))
		for _, t := range existing.Tags {
			attrs.Tags = append(attrs.Tags, t.Name)
		}
	}
	if !has("db_conn_string") {
		attrs.DBConnString = existing.DBConnString
	}
	if !has("db_source_type") {
		attrs.DBSourceType = existing.DBSourceType
	}
	if !has("db_conn_api_key") {
		// GetDatasourceByID hands back the $SECRET/ reference or the raw
		// key; either is the correct thing to write straight back.
		attrs.DBConnAPIKey = existing.DBConnAPIKey
	}
	if !has("db_name") {
		attrs.DBName = existing.DBName
	}
	if !has("namespace") {
		attrs.Namespace = existing.Namespace
	}
}

// mergeToolPatch keeps the tool's stored value for every attribute the body
// left out.
func mergeToolPatch(input *ToolInput, existing *models.Tool, present map[string]json.RawMessage) {
	attrs := &input.Data.Attributes
	has := func(k string) bool { _, ok := present[k]; return ok }
	if !has("name") {
		attrs.Name = existing.Name
	}
	if !has("description") {
		attrs.Description = existing.Description
	}
	if !has("tool_type") {
		attrs.ToolType = existing.ToolType
	}
	if !has("oas_spec") {
		attrs.OASSpec = existing.OASSpec
	}
	if !has("privacy_score") {
		attrs.PrivacyScore = existing.PrivacyScore
	}
	if !has("auth_key") {
		// GetToolByID keeps the key as stored (a reference or the key).
		attrs.AuthKey = existing.AuthKey
	}
	if !has("auth_schema_name") {
		attrs.AuthSchemaName = existing.AuthSchemaName
	}
	if !has("operations") {
		attrs.Operations = existing.GetOperations()
	}
	if !has("namespace") {
		attrs.Namespace = existing.Namespace
	}
}

// mergeEmbedderPatch keeps the embedder's stored value for every attribute
// the body left out.
func mergeEmbedderPatch(input *EmbedderInput, existing *models.Embedder, present map[string]json.RawMessage) {
	attrs := &input.Data.Attributes
	has := func(k string) bool { _, ok := present[k]; return ok }
	if !has("name") {
		attrs.Name = existing.Name
	}
	if !has("description") {
		attrs.Description = existing.Description
	}
	if !has("llm_id") {
		attrs.LLMID = existing.LLMID
	}
	if !has("vendor") {
		attrs.Vendor = string(existing.Vendor)
	}
	if !has("endpoint") {
		attrs.Endpoint = existing.Endpoint
	}
	if !has("api_key") {
		attrs.APIKey = services.RedactedEmbedderKey // keeps the stored key
	}
	if !has("model") {
		attrs.Model = existing.ModelName
	}
	if !has("privacy_score") {
		attrs.PrivacyScore = existing.PrivacyScore
	}
}
