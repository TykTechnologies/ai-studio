package services

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

const (
	provOpenID = "openid-connect"
	provLDAP   = "ldap"
	provSocial = "social"
)

func setProviderType(profile *models.Profile) error {
	if profile.ProviderName == "ADProvider" {
		profile.SelectedProviderType = provLDAP
		return nil
	}

	accessor := helpers.NewJSONMapAccessor(profile.ProviderConfig)
	useProviders := accessor.GetSlice("UseProviders")

	if len(useProviders) == 0 {
		return errors.New("no providers found")
	}

	provider, ok := useProviders[0].(map[string]interface{})
	if !ok {
		return errors.New("invalid provider")
	}

	providerAccessor := helpers.NewJSONMapAccessor(provider)
	providerName := providerAccessor.GetString("Name", "")

	if providerName == provOpenID {
		profile.SelectedProviderType = provOpenID
		return nil
	}

	profile.SelectedProviderType = provSocial
	return nil
}

func (s *Service) ValidateProfile(profile *models.Profile, userID uint, validateProfileID bool) error {
	if profile.ProfileID == "" && profile.Name == "" {
		return helpers.NewBadRequestError("name is required")
	}

	if profile.ProfileID == "" && profile.Name != "" {
		profile.ProfileID = strings.NewReplacer(" ", "-").Replace(strings.ToLower(profile.Name))
	}

	if validateProfileID {
		existingProfile := models.NewProfile()
		err := existingProfile.Get(s.DB, profile.ProfileID)

		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("Failed to check if profile exists", "profileID", profile.ProfileID, "error", err)
			return helpers.NewInternalServerError(fmt.Sprintf("error checking profile existence: %v", err))
		}

		if existingProfile.ID > 0 {
			return helpers.NewBadRequestError("profile ID already exists")
		}
	}

	if profile.DefaultUserGroupID != "" {
		groupID, err := strconv.ParseUint(profile.DefaultUserGroupID, 10, 64)
		if err != nil {
			return helpers.NewBadRequestError("invalid default user group ID")
		}

		group := models.NewGroup()
		if err := group.Get(s.DB, uint(groupID)); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return helpers.NewNotFoundError("default user group not found")
			}

			slog.Error("Failed to get default user group", "groupID", profile.DefaultUserGroupID, "error", err)
			return helpers.NewInternalServerError(fmt.Sprintf("error getting default user group: %v", err))
		}
	}

	for _, groupIDStr := range profile.UserGroupMapping {
		groupID, err := strconv.ParseUint(groupIDStr, 10, 64)
		if err != nil {
			return helpers.NewBadRequestError("invalid user group ID in mapping")
		}

		group := models.NewGroup()
		if err := group.Get(s.DB, uint(groupID)); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return helpers.NewNotFoundError("user group in mapping not found")
			}

			slog.Error("Failed to get user group from mapping", "groupID", groupIDStr, "error", err)
			return helpers.NewInternalServerError(fmt.Sprintf("error getting user group from mapping: %v", err))
		}
	}

	profile.UserID = userID

	if err := setProviderType(profile); err != nil {
		slog.Error("Failed to set provider type", "profileID", profile.ProfileID, "error", err)
		return helpers.NewInternalServerError(fmt.Sprintf("error setting provider type: %v", err))
	}

	return nil
}

func (s *Service) CreateProfile(profile *models.Profile, userID uint) error {
	if err := s.ValidateProfile(profile, userID, true); err != nil {
		return err
	}

	if err := profile.Create(s.DB); err != nil {
		slog.Error("Failed to create profile", "profileID", profile.ProfileID, "error", err)
		return helpers.NewInternalServerError(fmt.Sprintf("error creating profile: %v", err))
	}

	return nil
}

func (s *Service) GetProfileByID(profileID string) (*models.Profile, error) {
	profile := models.NewProfile()
	if err := profile.Get(s.DB, profileID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, helpers.NewNotFoundError(fmt.Sprintf("profile with ID %s not found", profileID))
		}

		slog.Error("Failed to get profile", "profileID", profileID, "error", err)
		return nil, helpers.NewInternalServerError(fmt.Sprintf("error getting profile: %v", err))
	}

	return profile, nil
}
func (s *Service) UpdateProfile(profileID string, updatedProfile *models.Profile, userID uint) (*models.Profile, error) {
	profile, err := s.GetProfileByID(profileID)
	if err != nil {
		return nil, err
	}

	originalProfileID := profile.ProfileID
	isProfileIDChanged := updatedProfile.ProfileID != originalProfileID

	profile.Name = updatedProfile.Name
	profile.OrgID = updatedProfile.OrgID
	profile.ActionType = updatedProfile.ActionType
	profile.MatchedPolicyID = updatedProfile.MatchedPolicyID
	profile.Type = updatedProfile.Type
	profile.ProviderName = updatedProfile.ProviderName
	profile.CustomEmailField = updatedProfile.CustomEmailField
	profile.CustomUserIDField = updatedProfile.CustomUserIDField
	profile.ProviderConfig = updatedProfile.ProviderConfig
	profile.IdentityHandlerConfig = updatedProfile.IdentityHandlerConfig
	profile.ProviderConstraintsDomain = updatedProfile.ProviderConstraintsDomain
	profile.ProviderConstraintsGroup = updatedProfile.ProviderConstraintsGroup
	profile.ReturnURL = updatedProfile.ReturnURL
	profile.DefaultUserGroupID = updatedProfile.DefaultUserGroupID
	profile.CustomUserGroupField = updatedProfile.CustomUserGroupField
	profile.UserGroupMapping = updatedProfile.UserGroupMapping
	profile.UserGroupSeparator = updatedProfile.UserGroupSeparator
	profile.SSOOnlyForRegisteredUsers = updatedProfile.SSOOnlyForRegisteredUsers

	if isProfileIDChanged {
		profile.ProfileID = updatedProfile.ProfileID
	}

	if err := s.ValidateProfile(profile, userID, isProfileIDChanged); err != nil {
		return nil, err
	}

	if err := profile.Update(s.DB); err != nil {
		slog.Error("Failed to update profile", "profileID", profileID, "error", err)
		return nil, helpers.NewInternalServerError(fmt.Sprintf("error updating profile: %v", err))
	}

	return profile, nil
}

func (s *Service) DeleteProfile(profileID string) error {
	profile, err := s.GetProfileByID(profileID)
	if err != nil {
		return err
	}

	if err := profile.Delete(s.DB); err != nil {
		slog.Error("Failed to delete profile", "profileID", profileID, "error", err)
		return helpers.NewInternalServerError(fmt.Sprintf("error deleting profile: %v", err))
	}

	return nil
}

func (s *Service) ListProfiles(pageSize int, pageNumber int, all bool) (models.Profiles, int64, int, error) {
	var profiles models.Profiles

	totalCount, totalPages, err := profiles.GetAll(s.DB, pageSize, pageNumber, all)
	if err != nil {
		slog.Error("Failed to list profiles", "error", err)
		return nil, 0, 0, helpers.NewInternalServerError(fmt.Sprintf("error listing profiles: %v", err))
	}

	return profiles, totalCount, totalPages, nil
}

func (s *Service) GetProfileByName(name string) (*models.Profile, error) {
	profile := models.NewProfile()

	if err := profile.GetByName(s.DB, name); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, helpers.NewNotFoundError(fmt.Sprintf("profile with name %s not found", name))
		}

		slog.Error("Failed to get profile by name", "name", name, "error", err)
		return nil, helpers.NewInternalServerError(fmt.Sprintf("error getting profile by name: %v", err))
	}

	return profile, nil
}
