package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// CatalogueGroup is one team that has been granted a catalogue, with its
// live member count, for the "teams using this catalogue" panel on the
// catalogue page and the delete confirmation.
type CatalogueGroup struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	MemberCount int64  `json:"member_count"`
}

// catalogueJoin names the many2many table and its catalogue column for each
// catalogue kind (models.Group).
var catalogueJoin = map[string][2]string{
	"catalogues":      {"group_catalogues", "catalogue_id"},
	"data-catalogues": {"group_datacatalogues", "data_catalogue_id"},
	"tool-catalogues": {"group_toolcatalogues", "tool_catalogue_id"},
}

// GetCatalogueGroups lists the teams that hold a catalogue, ordered by name,
// with the count of their non-deleted members. kind is "catalogues",
// "data-catalogues" or "tool-catalogues". Never nil: an unshared catalogue
// yields an empty slice.
func (s *Service) GetCatalogueGroups(kind string, catalogueID uint) ([]CatalogueGroup, error) {
	join, ok := catalogueJoin[kind]
	if !ok {
		return nil, fmt.Errorf("unknown catalogue kind %q", kind)
	}
	table, column := join[0], join[1]

	groups := []CatalogueGroup{}
	err := s.DB.Model(&models.Group{}).
		Select("groups.id AS id, groups.name AS name, COUNT(DISTINCT users.id) AS member_count").
		Joins("JOIN "+table+" ON "+table+".group_id = groups.id").
		Joins("LEFT JOIN user_groups ON user_groups.group_id = groups.id").
		Joins("LEFT JOIN users ON users.id = user_groups.user_id AND users.deleted_at IS NULL").
		Where(table+"."+column+" = ?", catalogueID).
		Group("groups.id, groups.name").
		Order("groups.name ASC, groups.id ASC").
		Scan(&groups).Error
	if err != nil {
		return nil, err
	}
	return groups, nil
}
