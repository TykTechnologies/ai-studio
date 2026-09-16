package models

import "gorm.io/gorm"

// Catalog membership for the portal's unified catalog: which of the caller's
// accessible catalogs each visible object belongs to. One query per object
// type, restricted to the accessible catalog ids, so the portal can say
// "available through ..." and offer the catalog as a filter without a lookup
// per item. The join tables are GORM many2many tables with no soft delete.

type catalogueMembershipRow struct {
	CatalogueID uint
	ItemID      uint
}

func catalogueMemberships(db *gorm.DB, table, catalogueCol, itemCol string, catalogueIDs []uint) (map[uint][]uint, error) {
	out := make(map[uint][]uint)
	if len(catalogueIDs) == 0 {
		return out, nil
	}
	var rows []catalogueMembershipRow
	err := db.Table(table).
		Select(catalogueCol+" AS catalogue_id, "+itemCol+" AS item_id").
		Where(catalogueCol+" IN ?", catalogueIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ItemID] = append(out[row.ItemID], row.CatalogueID)
	}
	return out, nil
}

// LLMCatalogueMemberships maps LLM id -> ids of the given catalogues it is in.
func LLMCatalogueMemberships(db *gorm.DB, catalogueIDs []uint) (map[uint][]uint, error) {
	return catalogueMemberships(db, "catalogue_llms", "catalogue_id", "llm_id", catalogueIDs)
}

// DatasourceCatalogueMemberships maps datasource id -> ids of the given data
// catalogues it is in.
func DatasourceCatalogueMemberships(db *gorm.DB, catalogueIDs []uint) (map[uint][]uint, error) {
	return catalogueMemberships(db, "data_catalogue_data_sources", "data_catalogue_id", "datasource_id", catalogueIDs)
}

// ToolCatalogueMemberships maps tool id -> ids of the given tool catalogues it
// is in.
func ToolCatalogueMemberships(db *gorm.DB, catalogueIDs []uint) (map[uint][]uint, error) {
	return catalogueMemberships(db, "tool_catalogue_tools", "tool_catalogue_id", "tool_id", catalogueIDs)
}

// Base queries for the objects a user can use, one per type. They are the
// GetAccessible* rules (the user's teams -> their catalogues -> active
// objects) as composable queries, so the portal catalog can add its filters
// (WHERE on the object's columns or the catalogue's name), count facets with
// aggregates, and order and page in SQL instead of loading every accessible
// row. The joins produce one row per (object, catalogue); callers group by
// the object's id or count DISTINCT ids.

func AccessibleLLMQuery(db *gorm.DB, userID uint) *gorm.DB {
	return db.Model(&LLM{}).
		Joins("JOIN catalogue_llms ON catalogue_llms.llm_id = llms.id").
		Joins("JOIN catalogues ON catalogues.id = catalogue_llms.catalogue_id").
		Joins("JOIN group_catalogues ON group_catalogues.catalogue_id = catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_catalogues.group_id").
		Where("user_groups.user_id = ? AND llms.active = ?", userID, true)
}

func AccessibleDatasourceQuery(db *gorm.DB, userID uint) *gorm.DB {
	return db.Model(&Datasource{}).
		Joins("JOIN data_catalogue_data_sources ON data_catalogue_data_sources.datasource_id = datasources.id").
		Joins("JOIN data_catalogues ON data_catalogues.id = data_catalogue_data_sources.data_catalogue_id").
		Joins("JOIN group_datacatalogues ON group_datacatalogues.data_catalogue_id = data_catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_datacatalogues.group_id").
		Where("user_groups.user_id = ? AND datasources.active = ?", userID, true)
}

// AccessibleToolQuery filters on tools.active, which the per-catalogue portal
// page also applies (GetAccessibleTools itself does not).
func AccessibleToolQuery(db *gorm.DB, userID uint) *gorm.DB {
	return db.Model(&Tool{}).
		Joins("JOIN tool_catalogue_tools ON tool_catalogue_tools.tool_id = tools.id").
		Joins("JOIN tool_catalogues ON tool_catalogues.id = tool_catalogue_tools.tool_catalogue_id").
		Joins("JOIN group_toolcatalogues ON group_toolcatalogues.tool_catalogue_id = tool_catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_toolcatalogues.group_id").
		Where("user_groups.user_id = ? AND tools.active = ?", userID, true)
}

// AccessibleMCPServerQuery is the portal visibility rule for Tyk-managed
// MCP servers: the user's teams -> direct team grants -> published servers
// that are active on their Dashboard. There is no catalogue family for
// this type; grants are per server.
func AccessibleMCPServerQuery(db *gorm.DB, userID uint) *gorm.DB {
	return db.Model(&MCPServer{}).
		Joins("JOIN tool_catalogue_mcp_servers ON tool_catalogue_mcp_servers.mcp_server_id = mcp_servers.id").
		Joins("JOIN tool_catalogues ON tool_catalogues.id = tool_catalogue_mcp_servers.tool_catalogue_id AND tool_catalogues.deleted_at IS NULL").
		Joins("JOIN group_toolcatalogues ON group_toolcatalogues.tool_catalogue_id = tool_catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_toolcatalogues.group_id").
		Where("user_groups.user_id = ? AND mcp_servers.is_active = ? AND mcp_servers.dashboard_state = ?", userID, true, MCPDashboardActive)
}
