package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateGroup(name string) (*models.Group, error) {
	group := &models.Group{
		Name: name,
	}

	if err := group.Create(s.DB); err != nil {
		return nil, err
	}

	return group, nil
}

func (s *Service) GetGroupByID(id uint) (*models.Group, error) {
	group := models.NewGroup()
	if err := group.Get(s.DB, id); err != nil {
		return nil, err
	}
	return group, nil
}

func (s *Service) UpdateGroup(id uint, name string) (*models.Group, error) {
	group, err := s.GetGroupByID(id)
	if err != nil {
		return nil, err
	}

	group.Name = name

	if err := group.Update(s.DB); err != nil {
		return nil, err
	}

	return group, nil
}

func (s *Service) DeleteGroup(id uint) error {
	group, err := s.GetGroupByID(id)
	if err != nil {
		return err
	}

	return group.Delete(s.DB)
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

func (s *Service) GetGroupUsers(groupID uint) (models.Users, error) {
	group, err := s.GetGroupByID(groupID)
	if err != nil {
		return nil, err
	}

	if err := group.GetGroupUsers(s.DB); err != nil {
		return nil, err
	}

	return group.Users, nil
}

func (s *Service) GetAllGroups() (models.Groups, error) {
	var groups models.Groups
	if err := groups.GetAll(s.DB); err != nil {
		return nil, err
	}
	return groups, nil
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
