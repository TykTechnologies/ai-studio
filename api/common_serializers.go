package api

import (
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// Common serialization functions used across multiple handlers
// These are NOT build-tagged so they're available in both CE and ENT builds

// serializeCatalogues serializes a list of LLM catalogues
func serializeCatalogues(catalogues models.Catalogues) []CatalogueResponse {
	result := make([]CatalogueResponse, len(catalogues))
	for i, catalogue := range catalogues {
		llmNames := make([]string, len(catalogue.LLMs))
		for j, llm := range catalogue.LLMs {
			llmNames[j] = llm.Name
		}
		result[i] = CatalogueResponse{
			Type: "catalogues",
			ID:   strconv.FormatUint(uint64(catalogue.ID), 10),
			Attributes: struct {
				Name     string   `json:"name"`
				LLMNames []string `json:"llm_names"`
			}{
				Name:     catalogue.Name,
				LLMNames: llmNames,
			},
		}
	}
	return result
}

// serializeDataCatalogues serializes a list of data catalogues
func serializeDataCatalogues(dataCatalogues models.DataCatalogues) []DataCatalogueResponse {
	result := make([]DataCatalogueResponse, len(dataCatalogues))
	for i, dataCatalogue := range dataCatalogues {
		datasourceResponses := make([]DatasourceResponse, len(dataCatalogue.Datasources))
		for j, datasource := range dataCatalogue.Datasources {
			datasourceResponses[j] = serializeDatasource(&datasource)
		}

		tagResponses := make([]TagResponse, len(dataCatalogue.Tags))
		for j, tag := range dataCatalogue.Tags {
			tagResponses[j] = TagResponse{
				Type: "tags",
				ID:   strconv.FormatUint(uint64(tag.ID), 10),
				Attributes: struct {
					Name string `json:"name"`
				}{
					Name: tag.Name,
				},
			}
		}

		result[i] = DataCatalogueResponse{
			Type: "data-catalogues",
			ID:   strconv.FormatUint(uint64(dataCatalogue.ID), 10),
			Attributes: struct {
				Name             string               `json:"name"`
				ShortDescription string               `json:"short_description"`
				LongDescription  string               `json:"long_description"`
				Icon             string               `json:"icon"`
				Datasources      []DatasourceResponse `json:"datasources"`
				Tags             []TagResponse        `json:"tags"`
			}{
				Name:             dataCatalogue.Name,
				ShortDescription: dataCatalogue.ShortDescription,
				LongDescription:  dataCatalogue.LongDescription,
				Icon:             dataCatalogue.Icon,
				Datasources:      datasourceResponses,
				Tags:             tagResponses,
			},
		}
	}
	return result
}

// serializeToolCatalogues serializes a list of tool catalogues
func serializeToolCatalogues(toolCatalogues models.ToolCatalogues, db *gorm.DB) []ToolCatalogueResponse {
	result := make([]ToolCatalogueResponse, len(toolCatalogues))
	for i, tc := range toolCatalogues {
		result[i] = ToolCatalogueResponse{
			Type: "tool-catalogues",
			ID:   strconv.FormatUint(uint64(tc.ID), 10),
			Attributes: struct {
				Name             string            `json:"name"`
				ShortDescription string            `json:"short_description"`
				LongDescription  string            `json:"long_description"`
				Icon             string            `json:"icon"`
				Tools            []ToolResponse    `json:"tools"`
				Tags             []TagResponse     `json:"tags"`
			}{
				Name:             tc.Name,
				ShortDescription: tc.ShortDescription,
				LongDescription:  tc.LongDescription,
				Icon:             tc.Icon,
				Tools:            serializeToolsSlim(tc.Tools, db),
				Tags:             serializeTags(tc.Tags),
			},
		}
	}
	return result
}

// serializeGroups serializes a list of groups (for user entitlements)
func serializeGroups(groups models.Groups) []GroupResponse {
	result := make([]GroupResponse, len(groups))
	for i, group := range groups {
		result[i] = GroupResponse{
			Type: "groups",
			ID:   strconv.FormatUint(uint64(group.ID), 10),
			Attributes: struct {
				Name           string                  `json:"name"`
				Users          []UserResponse          `json:"users,omitempty"`
				Catalogues     []CatalogueResponse     `json:"catalogues,omitempty"`
				DataCatalogues []DataCatalogueResponse `json:"data_catalogues,omitempty"`
				ToolCatalogues []ToolCatalogueResponse `json:"tool_catalogues,omitempty"`
			}{
				Name: group.Name,
			},
		}
	}
	return result
}
