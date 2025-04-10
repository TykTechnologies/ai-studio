package services

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
)

func (s *Service) CreateUser(email, name, password string, isAdmin bool, showChat bool, showPortal bool, emailVerified bool, notificationsEnabled bool, accessToSSOConfig bool) (*models.User, error) {
	// Only allow notifications and SSO config access if user is admin
	if notificationsEnabled && !isAdmin {
		return nil, fmt.Errorf("notifications can only be enabled for admin users")
	}

	// Only allow access to SSO config if user is admin
	if accessToSSOConfig && !isAdmin {
		return nil, fmt.Errorf("access to IdP configuration can only be enabled for admin users")
	}

	user := &models.User{
		Email:                email,
		Name:                 name,
		IsAdmin:              isAdmin,
		ShowChat:             showChat,
		ShowPortal:           showPortal,
		EmailVerified:        emailVerified,
		NotificationsEnabled: notificationsEnabled,
		AccessToSSOConfig:    accessToSSOConfig,
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

func (a *Service) GetUserByAPIKey(apiKey string) (*models.User, error) {
	user := models.NewUser()
	if err := user.GetByAPIKey(a.DB, apiKey); err != nil {
		return nil, err
	}
	return user, nil
}

func (a *Service) GenerateAPIKeyForUser(id uint) error {
	user, err := a.GetUserByID(id)
	if err != nil {
		return err
	}

	if err := user.GenerateAPIKey(); err != nil {
		return err
	}

	return user.Update(a.DB)
}

func (s *Service) GetUserByEmail(email string) (*models.User, error) {
	user := models.NewUser()
	if err := user.GetByEmail(s.DB, email); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) UpdateUser(id uint, email, name string, isAdmin bool, showChat bool, showPortal bool, emailVerified bool, notificationsEnabled bool, accessToSSOConfig bool) (*models.User, error) {
	user, err := s.GetUserByID(id)
	if err != nil {
		return nil, err
	}

	// Only allow notifications and SSO config access if user is admin
	if notificationsEnabled && !isAdmin {
		return nil, fmt.Errorf("notifications can only be enabled for admin users")
	}

	// Only allow access to SSO config if user is admin
	if accessToSSOConfig && !isAdmin {
		return nil, fmt.Errorf("access to IdP configuration can only be enabled for admin users")
	}

	user.Email = email
	user.Name = name
	user.IsAdmin = isAdmin
	user.ShowChat = showChat
	user.ShowPortal = showPortal
	user.EmailVerified = emailVerified
	user.NotificationsEnabled = notificationsEnabled
	user.AccessToSSOConfig = accessToSSOConfig

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

var (
	LoginFailedError      = errors.New("login failed")
	EmailNotVerifiedError = errors.New("email not verified")
)

func (s *Service) AuthenticateUser(email, password string) (*models.User, error) {
	user := models.NewUser()
	if err := user.GetByEmail(s.DB, email); err != nil {
		return nil, err
	}

	if !user.DoesPasswordMatch(password) {
		return nil, LoginFailedError
	}

	if !user.EmailVerified {
		return nil, EmailNotVerifiedError
	}

	return user, nil
}

func (s *Service) GetAllUsers(pageSize, pageNumber int, all bool, sort string) (models.Users, int64, int, error) {
	var users models.Users
	totalCount, totalPages, err := users.GetAll(s.DB, pageSize, pageNumber, all, sort)
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

type UserEntitlements struct {
	User           *models.User
	Catalogues     []models.Catalogue
	DataCatalogues []models.DataCatalogue
	ToolCatalogues []models.ToolCatalogue
	Chats          []models.Chat
}

func (ue *UserEntitlements) HasDataSourceAccess(dataSourceID uint) bool {
	// Admins have access to everything
	if ue.User.IsAdmin {
		return true
	}

	// For regular users, check each data catalogue
	for _, dc := range ue.DataCatalogues {
		// Check each datasource in the catalogue
		for _, dataSource := range dc.Datasources {
			if dataSource.ID == dataSourceID {
				return true
			}
		}
	}

	// If we haven't found the datasource in any catalogue, the user doesn't have access
	return false
}

func (ue *UserEntitlements) HasToolAccess(toolID uint) bool {
	// Admins have access to everything
	if ue.User.IsAdmin {
		return true
	}

	// For regular users, check each tool catalogue
	for _, tc := range ue.ToolCatalogues {
		// Check each tool in the catalogue
		for _, tool := range tc.Tools {
			if tool.ID == toolID {
				return true
			}
		}
	}

	// If we haven't found the tool in any catalogue, the user doesn't have access
	return false
}

// GetUserEntitlements retrieves all entitlements for a given user
func (s *Service) GetUserEntitlements(userID uint) (*UserEntitlements, error) {
	// Get user
	user, err := s.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Get user's groups
	groups, err := s.GetGroupsByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user groups: %w", err)
	}

	// Use maps to ensure uniqueness
	catalogues := make(map[uint]models.Catalogue)
	dataCatalogues := make(map[uint]models.DataCatalogue)
	toolCatalogues := make(map[uint]models.ToolCatalogue)
	chats := make(map[uint]models.Chat)

	for _, group := range groups {
		// Get catalogues for this group
		groupCatalogues, err := s.GetGroupCatalogues(group.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get group catalogues: %w", err)
		}
		for _, catalogue := range groupCatalogues {
			catalogues[catalogue.ID] = catalogue
		}

		// Get data catalogues for this group
		groupDataCatalogues, err := s.GetGroupDataCatalogues(group.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get group data catalogues: %w", err)
		}
		for _, dataCatalogue := range groupDataCatalogues {
			dataCatalogues[dataCatalogue.ID] = dataCatalogue
		}

		// Get tool catalogues for this group
		groupToolCatalogues, _, _, err := s.GetGroupToolCatalogues(group.ID, 1, 1, true)
		if err != nil {
			return nil, fmt.Errorf("failed to get group tool catalogues: %w", err)
		}
		for _, toolCatalogue := range groupToolCatalogues {
			toolCatalogues[toolCatalogue.ID] = toolCatalogue
		}

		// Get chats for this group
		groupChats, err := s.GetChatsByGroupID(group.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get group chats: %w", err)
		}
		for _, chat := range groupChats {
			chats[chat.ID] = chat
		}
	}

	// Convert maps to slices
	entitlements := &UserEntitlements{
		User:           user,
		Catalogues:     mapToSlice(catalogues),
		DataCatalogues: mapToSlice(dataCatalogues),
		ToolCatalogues: mapToSlice(toolCatalogues),
		Chats:          mapToSlice(chats),
	}

	return entitlements, nil
}

// Helper function to convert map to slice
func mapToSlice[T any](m map[uint]T) []T {
	slice := make([]T, 0, len(m))
	for _, v := range m {
		slice = append(slice, v)
	}
	return slice
}
