package services

import (
	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateUser(email, name, password string, isAdmin bool, showChat bool, showPortal bool) (*models.User, error) {
	user := &models.User{
		Email:      email,
		Name:       name,
		IsAdmin:    isAdmin,
		ShowChat:   showChat,
		ShowPortal: showPortal,
	}

	if err := user.SetPassword(password); err != nil {
		return nil, err
	}

	if err := user.Create(s.DB); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) GetUserByID(id uint) (*models.User, error) {
	user := models.NewUser()
	if err := user.Get(s.DB, id); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) GetUserByEmail(email string) (*models.User, error) {
	user := models.NewUser()
	if err := user.GetByEmail(s.DB, email); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) UpdateUser(id uint, email, name string, isAdmin bool, showChat bool, showPortal bool) (*models.User, error) {
	user, err := s.GetUserByID(id)
	if err != nil {
		return nil, err
	}

	user.Email = email
	user.Name = name
	user.IsAdmin = isAdmin
	user.ShowChat = showChat
	user.ShowPortal = showPortal

	if err := user.Update(s.DB); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) DeleteUser(id uint) error {
	user, err := s.GetUserByID(id)
	if err != nil {
		return err
	}

	return user.Delete(s.DB)
}

func (s *Service) AuthenticateUser(email, password string) (*models.User, error) {
	user := models.NewUser()
	if err := user.GetByEmail(s.DB, email); err != nil {
		return nil, err
	}

	if !user.DoesPasswordMatch(password) {
		return nil, errors.New("invalid password")
	}

	return user, nil
}

func (s *Service) GetAllUsers(pageSize, pageNumber int, all bool) (models.Users, int64, int, error) {
	var users models.Users
	totalCount, totalPages, err := users.GetAll(s.DB, pageSize, pageNumber, all)
	if err != nil {
		return nil, 0, 0, err
	}
	return users, totalCount, totalPages, nil
}

func (s *Service) SearchUsersByEmailStub(stub string) (models.Users, error) {
	var users models.Users
	if err := users.SearchByEmailStub(s.DB, stub); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Service) GetUserAccessibleCatalogues(userID uint) (models.Catalogues, error) {
	user, err := s.GetUserByID(userID)
	if err != nil {
		return nil, err
	}

	catalogues, err := user.GetAccessibleCatalogues(s.DB)
	if err != nil {
		return nil, err
	}

	return catalogues, nil
}

func (s *Service) GetAccessibleToolsForUser(userID uint) ([]models.Tool, error) {
	user := &models.User{ID: userID}
	return user.GetAccessibleTools(s.DB)
}
