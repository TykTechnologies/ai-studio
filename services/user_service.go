package services

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

type UserDTO struct {
	Email                string
	Name                 string
	Password             string
	IsAdmin              bool
	ShowChat             bool
	ShowPortal           bool
	EmailVerified        bool
	NotificationsEnabled bool
	AccessToSSOConfig    bool
	Groups               []uint
}

func (s *Service) addDefaultGroupIfNotExists(groups []uint) ([]uint, error) {
	// Get or create default group by name (safe for any DB state)
	defaultGroup, err := models.GetOrCreateDefaultGroup(s.DB)
	if err != nil {
		return groups, err
	}

	// Check if user already has default group
	if !slices.Contains(groups, defaultGroup.ID) {
		groups = append(groups, defaultGroup.ID)
	}

	return groups, nil
}

func (s *Service) validateUserInput(dto UserDTO) error {
	if err := helpers.ValidateEmailDomain(dto.Email); err != nil {
		return err
	}

	if dto.NotificationsEnabled && !dto.IsAdmin {
		return helpers.NewBadRequestError("notifications can only be enabled for admin users")
	}

	if dto.AccessToSSOConfig && !dto.IsAdmin {
		return helpers.NewBadRequestError("access to IdP configuration can only be enabled for admin users")
	}

	if len(dto.Groups) > 0 {
		groupsExist, err := models.ValidateGroupsExist(s.DB, dto.Groups)
		if err != nil {
			return err
		}

		if !groupsExist {
			return helpers.NewBadRequestError("groups not found")
		}
	}

	return nil
}

func (s *Service) CreateUser(dto UserDTO) (*models.User, error) {
	if err := s.validateUserInput(dto); err != nil {
		return nil, err
	}

	user := &models.User{
		Email:                dto.Email,
		Name:                 dto.Name,
		IsAdmin:              dto.IsAdmin,
		ShowChat:             dto.ShowChat,
		ShowPortal:           dto.ShowPortal,
		EmailVerified:        dto.EmailVerified,
		NotificationsEnabled: dto.NotificationsEnabled,
		AccessToSSOConfig:    dto.AccessToSSOConfig,
	}

	if err := user.SetPassword(dto.Password); err != nil {
		return nil, err
	}

	groups, err := s.addDefaultGroupIfNotExists(dto.Groups)
	if err != nil {
		return nil, err
	}

	if len(groups) > 0 {
		user.ParseGroupAssociations(groups)
	}

	// Execute "before_create" hooks
	if s.HookManager != nil {
		hookResult, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeUser,
			HookBeforeCreate,
			user,
			0, // No user context for user creation
		)
		if err != nil {
			return nil, fmt.Errorf("hook execution failed: %w", err)
		}

		// Check if operation was rejected
		if !hookResult.Allowed {
			return nil, fmt.Errorf("operation rejected by plugin: %s", hookResult.RejectionReason)
		}

		// Use modified object if hooks modified it
		if hookResult.ModifiedObject != nil {
			if modified, ok := hookResult.ModifiedObject.(*models.User); ok {
				user = modified
			}
		}

		// Merge plugin metadata
		if err := s.HookManager.MergeMetadata(user, hookResult.Metadata); err != nil {
			logger.Warn(fmt.Sprintf("Failed to merge hook metadata: %v", err))
		}
	}

	if err := user.Create(s.DB); err != nil {
		return nil, err
	}

	// Execute "after_create" hooks
	if s.HookManager != nil {
		_, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeUser,
			HookAfterCreate,
			user,
			uint32(user.ID),
		)
		if err != nil {
			// Log but don't fail the operation
			logger.Warn(fmt.Sprintf("After-create hooks failed: %v", err))
		}
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitUserCreated(user, user.ID, 0)
	}

	return user, nil
}

func (s *Service) GetUserByID(id uint, preload ...string) (*models.User, error) {
	user := models.NewUser()
	if err := user.Get(s.DB, id, preload...); err != nil {
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

func (s *Service) UpdateUser(user *models.User, dto UserDTO) (*models.User, error) {
	if err := s.validateUserInput(dto); err != nil {
		return nil, err
	}

	user.Email = dto.Email
	user.Name = dto.Name
	user.IsAdmin = dto.IsAdmin
	user.ShowChat = dto.ShowChat
	user.ShowPortal = dto.ShowPortal
	user.EmailVerified = dto.EmailVerified
	user.NotificationsEnabled = dto.NotificationsEnabled
	user.AccessToSSOConfig = dto.AccessToSSOConfig

	newGroups := user.GetGroupsToUpdate(dto.Groups)

	// Execute "before_update" hooks
	if s.HookManager != nil {
		hookResult, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeUser,
			HookBeforeUpdate,
			user,
			uint32(user.ID),
		)
		if err != nil {
			return nil, fmt.Errorf("hook execution failed: %w", err)
		}

		// Check if operation was rejected
		if !hookResult.Allowed {
			return nil, fmt.Errorf("operation rejected by plugin: %s", hookResult.RejectionReason)
		}

		// Use modified object if hooks modified it
		if hookResult.ModifiedObject != nil {
			if modified, ok := hookResult.ModifiedObject.(*models.User); ok {
				user = modified
			}
		}

		// Merge plugin metadata
		if err := s.HookManager.MergeMetadata(user, hookResult.Metadata); err != nil {
			logger.Warn(fmt.Sprintf("Failed to merge hook metadata: %v", err))
		}
	}

	tx := s.DB.Begin()

	if err := user.Update(tx); err != nil {
		tx.Rollback()
		return nil, err
	}

	if newGroups != nil {
		if err := user.ReplaceGroupAssociation(tx, newGroups); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// Execute "after_update" hooks
	if s.HookManager != nil {
		_, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeUser,
			HookAfterUpdate,
			user,
			uint32(user.ID),
		)
		if err != nil {
			// Log but don't fail the operation
			logger.Warn(fmt.Sprintf("After-update hooks failed: %v", err))
		}
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitUserUpdated(user, user.ID, user.ID)
	}

	return user, nil
}

func (s *Service) DeleteUser(user *models.User) error {
	if user.GetRole() == models.RoleSuperAdmin {
		return helpers.NewForbiddenError("super admin user cannot be deleted")
	}

	// Execute "before_delete" hooks
	if s.HookManager != nil {
		hookResult, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeUser,
			HookBeforeDelete,
			user,
			uint32(user.ID),
		)
		if err != nil {
			return fmt.Errorf("hook execution failed: %w", err)
		}

		// Check if operation was rejected
		if !hookResult.Allowed {
			return fmt.Errorf("operation rejected by plugin: %s", hookResult.RejectionReason)
		}
	}

	tx := s.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	// Get all apps owned by this user and deactivate their credentials
	var userApps []models.App
	if err := tx.Where("user_id = ?", user.ID).Find(&userApps).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to find user's apps: %w", err)
	}

	// Deactivate credentials for all user's apps and mark them as orphaned
	for _, app := range userApps {
		// Mark the app as orphaned since its user is being deleted
		if err := tx.Model(&app).Update("is_orphaned", true).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to mark app %d as orphaned: %w", app.ID, err)
		}

		if app.CredentialID != 0 {
			var credential models.Credential
			if err := tx.First(&credential, app.CredentialID).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					tx.Rollback()
					return fmt.Errorf("failed to find credential for app %d: %w", app.ID, err)
				}
				// If credential doesn't exist, continue with next app
				continue
			}

			// Deactivate the credential to revoke access
			if err := credential.Deactivate(tx); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to deactivate credential for app %d: %w", app.ID, err)
			}
		}
	}

	if err := user.DeleteGroupAssociation(tx); err != nil {
		tx.Rollback()
		return err
	}

	if err := user.Delete(tx); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	// Execute "after_delete" hooks
	if s.HookManager != nil {
		_, err := s.HookManager.ExecuteHooks(
			context.Background(),
			ObjectTypeUser,
			HookAfterDelete,
			user,
			uint32(user.ID),
		)
		if err != nil {
			// Log but don't fail the operation
			logger.Warn(fmt.Sprintf("After-delete hooks failed: %v", err))
		}
	}

	// Emit system event
	if s.SystemEvents != nil {
		s.SystemEvents.EmitUserDeleted(user.ID, 0)
	}

	return nil
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
// CE: Returns all catalogues (no filtering)
// ENT: Returns catalogues filtered by user's group memberships
func (s *Service) GetUserEntitlements(userID uint) (*UserEntitlements, error) {
	// Get user
	user, err := s.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Delegate to GroupAccessService (handles CE/ENT split)
	baseEntitlements, err := s.GroupAccessService.GetUserEntitlements(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entitlements: %w", err)
	}

	// Convert to UserEntitlements (includes user object)
	return &UserEntitlements{
		User:           user,
		Catalogues:     baseEntitlements.Catalogues,
		DataCatalogues: baseEntitlements.DataCatalogues,
		ToolCatalogues: baseEntitlements.ToolCatalogues,
		Chats:          baseEntitlements.Chats,
	}, nil
}

// Helper function to convert map to slice
func mapToSlice[T any](m map[uint]T) []T {
	slice := make([]T, 0, len(m))
	for _, v := range m {
		slice = append(slice, v)
	}
	return slice
}

func (s *Service) SkipQuickStartForUser(userID uint) error {
	return models.SetSkipQuickStartForUser(s.DB, userID)
}

type ListUsersParams struct {
	Search         string
	ExcludeGroupID int
	PageSize       int
	PageNumber     int
	All            bool
	Sort           string
}

func (s *Service) ListUsers(params ListUsersParams) (models.Users, int64, int, error) {
	var users models.Users

	modelParams := models.UserQueryParams{
		Search:         params.Search,
		ExcludeGroupID: uint(params.ExcludeGroupID),
		PageSize:       params.PageSize,
		PageNumber:     params.PageNumber,
		All:            params.All,
		Sort:           params.Sort,
	}

	totalCount, totalPages, err := users.QueryUsers(s.DB, modelParams)
	if err != nil {
		return nil, 0, 0, err
	}

	return users, totalCount, totalPages, nil
}

// GetAllUsers is a wrapper for ListUsers for backward compatibility
func (s *Service) GetAllUsers(pageSize int, pageNumber int, all bool, sort string) (models.Users, int64, int, error) {
	params := ListUsersParams{
		PageSize:   pageSize,
		PageNumber: pageNumber,
		All:        all,
		Sort:       sort,
	}
	return s.ListUsers(params)
}

func (s *Service) UpdateGroupUsers(id uint, userIDs []uint) error {
	group, err := s.GetGroupByID(id, "Users")
	if err != nil {
		return err
	}

	tx := s.DB.Begin()

	users := make([]models.User, 0, len(userIDs))
	for _, userID := range userIDs {
		users = append(users, models.User{ID: userID})
	}

	if err := group.ReplaceAssociation(tx, "Users", users); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}
