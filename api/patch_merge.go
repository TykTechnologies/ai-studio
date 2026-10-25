package api

import (
	"encoding/json"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// The stored record as each resource's PATCH input, laid under the body by
// patchOnto (patch.go). Attributes that are already pointers (omitted =
// unchanged) keep the request's value and are left to their handlers.

// mergeDatasourcePatch keeps the datasource's stored value for every
// attribute the body left out. The embed_* fields already keep the current
// embedder's values when empty (pickDatasourceEmbedder), and active is
// handled by updateDatasource.
func mergeDatasourcePatch(input *DatasourceInput, existing *models.Datasource, present map[string]json.RawMessage) error {
	base := input.Data.Attributes
	base.Name = existing.Name
	base.ShortDescription = existing.ShortDescription
	base.LongDescription = existing.LongDescription
	base.Icon = existing.Icon
	base.Url = existing.Url
	base.PrivacyScore = existing.PrivacyScore
	base.UserID = existing.UserID
	base.Tags = make([]string, 0, len(existing.Tags))
	for _, t := range existing.Tags {
		base.Tags = append(base.Tags, t.Name)
	}
	base.DBConnString = existing.DBConnString
	base.DBSourceType = existing.DBSourceType
	// GetDatasourceByID hands back the $SECRET/ reference or the raw
	// key; either is the correct thing to write straight back.
	base.DBConnAPIKey = existing.DBConnAPIKey
	base.DBName = existing.DBName
	base.Namespace = existing.Namespace
	return patchOnto(&input.Data.Attributes, base, present)
}

// mergeToolPatch keeps the tool's stored value for every attribute the body
// left out.
func mergeToolPatch(input *ToolInput, existing *models.Tool, present map[string]json.RawMessage) error {
	base := input.Data.Attributes
	base.Name = existing.Name
	base.Description = existing.Description
	base.ToolType = existing.ToolType
	base.OASSpec = existing.OASSpec
	base.PrivacyScore = existing.PrivacyScore
	// GetToolByID keeps the key as stored (a reference or the key).
	base.AuthKey = existing.AuthKey
	base.AuthSchemaName = existing.AuthSchemaName
	base.Operations = existing.GetOperations()
	base.Namespace = existing.Namespace
	return patchOnto(&input.Data.Attributes, base, present)
}

// mergeEmbedderPatch keeps the embedder's stored value for every attribute
// the body left out.
func mergeEmbedderPatch(input *EmbedderInput, existing *models.Embedder, present map[string]json.RawMessage) error {
	base := input.Data.Attributes
	base.Name = existing.Name
	base.Description = existing.Description
	base.LLMID = existing.LLMID
	base.Vendor = string(existing.Vendor)
	base.Endpoint = existing.Endpoint
	base.APIKey = services.RedactedEmbedderKey // keeps the stored key
	base.Model = existing.ModelName
	base.PrivacyScore = existing.PrivacyScore
	return patchOnto(&input.Data.Attributes, base, present)
}
