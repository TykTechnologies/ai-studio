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
