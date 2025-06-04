package services

import (
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) validateGroupName(name string, groupID uint) error {
	if name == "" {
		return helpers.NewBadRequestError("group name is required")
	}

	isUnique, err := models.IsGroupNameUnique(s.DB, name, groupID)
	if err != nil {
		return helpers.NewInternalServerError("error checking group name uniqueness: " + err.Error())
	}

	if !isUnique {
		return helpers.NewBadRequestError("group name already exists")
	}

	return nil
}

func (s *Service) CreateGroup(name string, userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs []uint) (*models.Group, error) {
	if err := s.validateGroupName(name, 0); err != nil {
		return nil, err
	}

	group := &models.Group{
		Name: name,
	}

	group.ParseAssociations(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

	if err := group.Create(s.DB); err != nil {
		return nil, err
	}

	return group, nil
}

func (s *Service) GetGroupByID(id uint, preloads ...string) (*models.Group, error) {
	group := models.NewGroup()
	if err := group.Get(s.DB, id, preloads...); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *Service) UpdateGroup(id uint, name string, userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs []uint) (*models.Group, error) {
	if err := s.validateGroupName(name, id); err != nil {
		return nil, err
	}

	group, err := s.GetGroupByID(id, "Users", "Catalogues", "DataCatalogues", "ToolCatalogues")
	if err != nil {
		return nil, err
	}

	tx := s.DB.Begin()

	if name != "" && name != group.Name {
		group.Name = name
		if err := group.Update(tx); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	associations := group.GetAssociationsToUpdate(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

	for _, assoc := range associations {
		if assoc.NeedsUpdate {
			if err := group.ReplaceAssociation(tx, assoc.Name, assoc.GetValue()); err != nil {
				tx.Rollback()
				return nil, err
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return group, nil
}

func (s *Service) DeleteGroup(id uint) error {
	group, err := s.GetGroupByID(id)
	if err != nil {
		return err
	}

	tx := s.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	if err := group.ClearAssociations(tx); err != nil {
		tx.Rollback()
		return err
	}

	if err := group.Delete(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

func (s *Service) AddUserToGroup(userID, groupID uint) error {
	user, err := s.GetUserByID(userID)
	if err != nil {
		return err
	}

	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	return group.AddUser(s.DB, user)
}

func (s *Service) RemoveUserFromGroup(userID, groupID uint) error {
	user, err := s.GetUserByID(userID)
	if err != nil {
		return err
	}

	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	return group.RemoveUser(s.DB, user)
}

func (s *Service) GetGroupUsers(groupID uint, pageSize int, pageNumber int, all bool) (models.Users, int64, int, error) {
	var users models.Users
	totalCount, totalPages, err := users.GetGroupUsersPaginated(s.DB, groupID, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}
	return users, totalCount, totalPages, nil
}

func (s *Service) GetAllGroups(pageSize int, pageNumber int, all bool, sort string) (models.Groups, int64, int, error) {
	var groups models.Groups
	totalCount, totalPages, err := groups.GetAll(s.DB, pageSize, pageNumber, all, sort, "Catalogues", "DataCatalogues", "ToolCatalogues")
	if err != nil {
		return nil, 0, 0, err
	}
	return groups, totalCount, totalPages, nil
}

func (s *Service) SearchGroups(term string, pageSize int, pageNumber int, all bool, sort string) (models.Groups, int64, int, error) {
	var groups models.Groups
	totalCount, totalPages, err := groups.SearchByTerm(s.DB, term, pageSize, pageNumber, all, sort, "Catalogues", "DataCatalogues", "ToolCatalogues")
	if err != nil {
		return nil, 0, 0, err
	}
	return groups, totalCount, totalPages, nil
}

func (s *Service) GetGroupsWithMemberCounts(term string, pageSize int, pageNumber int, all bool, sort string) (models.Groups, []models.GroupMemberCount, int64, int, error) {
	var groups models.Groups
	var totalCount int64
	var totalPages int
	var err error

	if term != "" {
		groups, totalCount, totalPages, err = s.SearchGroups(term, pageSize, pageNumber, all, sort)
	} else {
		groups, totalCount, totalPages, err = s.GetAllGroups(pageSize, pageNumber, all, sort)
	}

	if err != nil {
		return nil, nil, 0, 0, err
	}

	memberCounts, err := groups.GetGroupsMemberCounts(s.DB)
	if err != nil {
		return nil, nil, 0, 0, err
	}

	return groups, memberCounts, totalCount, totalPages, nil
}

func (s *Service) SearchGroupsByNameStub(stub string) (models.Groups, error) {
	var groups models.Groups
	if err := groups.GetByNameStub(s.DB, stub); err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *Service) AddCatalogueToGroup(catalogueID, groupID uint) error {
	catalogue, err := s.GetCatalogueByID(catalogueID)
	if err != nil {
		return err
	}

	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	return group.AddCatalogue(s.DB, catalogue)
}

func (s *Service) RemoveCatalogueFromGroup(catalogueID, groupID uint) error {
	catalogue, err := s.GetCatalogueByID(catalogueID)
	if err != nil {
		return err
	}

	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	return group.RemoveCatalogue(s.DB, catalogue)
}

func (s *Service) GetGroupCatalogues(groupID uint) (models.Catalogues, error) {
	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return nil, err
	}

	if err := group.GetCatalogues(s.DB); err != nil {
		return nil, err
	}

	return group.Catalogues, nil
}

func (s *Service) GetGroupsByUserID(userID uint) (models.Groups, error) {
	var groups models.Groups
	err := groups.GetGroupsByUserID(s.DB, userID)
	if err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *Service) AddDataCatalogueToGroup(dataCatalogueID, groupID uint) error {
	dataCatalogue, err := s.GetDataCatalogueByID(dataCatalogueID)
	if err != nil {
		return err
	}

	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	return group.AddDataCatalogue(s.DB, dataCatalogue)
}

func (s *Service) RemoveDataCatalogueFromGroup(dataCatalogueID, groupID uint) error {
	dataCatalogue, err := s.GetDataCatalogueByID(dataCatalogueID)
	if err != nil {
		return err
	}

	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	return group.RemoveDataCatalogue(s.DB, dataCatalogue)
}

func (s *Service) GetGroupDataCatalogues(groupID uint) (models.DataCatalogues, error) {
	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return nil, err
	}

	if err := group.GetDataCatalogues(s.DB); err != nil {
		return nil, err
	}

	return group.DataCatalogues, nil
}

func (s *Service) AddToolCatalogueToGroup(toolCatalogueID, groupID uint) error {
	toolCatalogue, err := s.GetToolCatalogueByID(toolCatalogueID)
	if err != nil {
		return err
	}

	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	return group.AddToolCatalogue(s.DB, toolCatalogue)
}

func (s *Service) RemoveToolCatalogueFromGroup(toolCatalogueID, groupID uint) error {
	toolCatalogue, err := s.GetToolCatalogueByID(toolCatalogueID)
	if err != nil {
		return err
	}

	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return err
	}

	return group.RemoveToolCatalogue(s.DB, toolCatalogue)
}

func (s *Service) GetGroupToolCatalogues(groupID uint, pageSize int, pageNumber int, all bool) (models.ToolCatalogues, int64, int, error) {
	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return nil, 0, 0, err
	}

	totalCount, totalPages, err := group.GetToolCatalogues(s.DB, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}

	return group.ToolCatalogues, totalCount, totalPages, nil
}

func (s *Service) UpdateGroupCatalogues(id uint, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs []uint) error {
	group, err := s.GetGroupByID(id, "Catalogues", "DataCatalogues", "ToolCatalogues")
	if err != nil {
		return err
	}

	associations := group.GetAssociationsToUpdate([]uint{}, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

	tx := s.DB.Begin()

	for _, assoc := range associations {
		if assoc.Name != "Users" && assoc.NeedsUpdate {
			if err := group.ReplaceAssociation(tx, assoc.Name, assoc.GetValue()); err != nil {
				tx.Rollback()
				return err
			}
		}
	}

	return tx.Commit().Error
}
