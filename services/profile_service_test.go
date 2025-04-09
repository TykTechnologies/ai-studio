package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupProfileTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Initialize models - this will create all necessary tables and relationships
	err = models.InitModels(db)
	require.NoError(t, err)

	// Migrate the Profile model explicitly
	err = db.AutoMigrate(&models.Profile{})
	require.NoError(t, err)

	// Create a default group
	defaultGroup := &models.Group{
		Name: "Default Group",
	}
	err = db.Create(defaultGroup).Error
	require.NoError(t, err)
	assert.Equal(t, uint(1), defaultGroup.ID) // Default group should have ID 1

	return db
}

func createTestProfileData() *models.Profile {
	return &models.Profile{
		ProfileID:                 "test-profile-id",
		Name:                      "Test Profile",
		OrgID:                     "test-org-id",
		ActionType:                "GenerateOrLoginUserProfile",
		MatchedPolicyID:           "test-policy-id",
		Type:                      "social",
		ProviderName:              "social",
		CustomEmailField:          "email",
		CustomUserIDField:         "id",
		ProviderConfig:            models.JSONMap{"UseProviders": []interface{}{map[string]interface{}{"Name": "social"}}},
		IdentityHandlerConfig:     models.JSONMap{"handler_type": "test-handler"},
		ProviderConstraintsDomain: "example.com",
		ProviderConstraintsGroup:  "test-group",
		ReturnURL:                 "https://example.com/return",
		DefaultUserGroupID:        "1",
		CustomUserGroupField:      "group",
		UserGroupMapping:          models.StringMap{"admin": "1", "user": "1"},
		UserGroupSeparator:        ",",
		SSOOnlyForRegisteredUsers: true,
	}
}

func createOpenIDProfileData() *models.Profile {
	return &models.Profile{
		ProfileID:             "openid-profile-id",
		Name:                  "OpenID Profile",
		OrgID:                 "test-org-id",
		ActionType:            "GenerateOrLoginUserProfile",
		MatchedPolicyID:       "test-policy-id",
		Type:                  "openid-connect",
		ProviderName:          "openid",
		ProviderConfig:        models.JSONMap{"UseProviders": []interface{}{map[string]interface{}{"Name": "openid-connect"}}},
		IdentityHandlerConfig: models.JSONMap{"handler_type": "test-handler"},
		DefaultUserGroupID:    "1",
	}
}

func createADProfileData() *models.Profile {
	return &models.Profile{
		ProfileID:             "ad-profile-id",
		Name:                  "AD Profile",
		OrgID:                 "test-org-id",
		ActionType:            "GenerateOrLoginUserProfile",
		MatchedPolicyID:       "test-policy-id",
		Type:                  "ldap",
		ProviderName:          "ADProvider",
		ProviderConfig:        models.JSONMap{},
		IdentityHandlerConfig: models.JSONMap{"handler_type": "test-handler"},
		DefaultUserGroupID:    "1",
	}
}

func createInvalidProviderProfileData() *models.Profile {
	return &models.Profile{
		ProfileID:             "invalid-profile-id",
		Name:                  "Invalid Profile",
		OrgID:                 "test-org-id",
		ActionType:            "GenerateOrLoginUserProfile",
		MatchedPolicyID:       "test-policy-id",
		Type:                  "social",
		ProviderName:          "social",
		ProviderConfig:        models.JSONMap{"UseProviders": []interface{}{}}, // Empty providers
		IdentityHandlerConfig: models.JSONMap{"handler_type": "test-handler"},
		DefaultUserGroupID:    "1",
	}
}

func TestSetProviderType(t *testing.T) {
	tests := []struct {
		name          string
		profile       *models.Profile
		expectedType  string
		expectError   bool
		errorContains string
	}{
		{
			name:         "Social provider",
			profile:      createTestProfileData(),
			expectedType: provSocial,
			expectError:  false,
		},
		{
			name:         "OpenID provider",
			profile:      createOpenIDProfileData(),
			expectedType: provOpenID,
			expectError:  false,
		},
		{
			name:         "AD provider",
			profile:      createADProfileData(),
			expectedType: provLDAP,
			expectError:  false,
		},
		{
			name:          "No providers",
			profile:       createInvalidProviderProfileData(),
			expectedType:  "",
			expectError:   true,
			errorContains: "no providers found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := setProviderType(tt.profile)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedType, tt.profile.SelectedProviderType)
			}
		})
	}
}

func TestService_ValidateProfile(t *testing.T) {
	// Run each test in a separate subtest with its own database
	t.Run("Valid profile", func(t *testing.T) {
		db := setupProfileTestDB(t)
		service := &Service{DB: db}
		profile := createTestProfileData()
		err := service.ValidateProfile(profile, 1, true)
		assert.NoError(t, err)
		assert.Equal(t, uint(1), profile.UserID)
		assert.Equal(t, provSocial, profile.SelectedProviderType)
	})

	t.Run("Missing name and ID", func(t *testing.T) {
		db := setupProfileTestDB(t)
		service := &Service{DB: db}
		profile := createTestProfileData()
		profile.ProfileID = ""
		profile.Name = ""
		err := service.ValidateProfile(profile, 1, true)
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Bad Request", errResp.Title)
		assert.Contains(t, errResp.Message, "name is required")
	})

	t.Run("Generate profile ID from name", func(t *testing.T) {
		db := setupProfileTestDB(t)
		service := &Service{DB: db}
		profile := createTestProfileData()
		profile.ProfileID = ""
		profile.Name = "Test Profile Name"
		err := service.ValidateProfile(profile, 1, true)
		assert.NoError(t, err)
		assert.Equal(t, "test-profile-name", profile.ProfileID)
	})

	t.Run("Profile ID already exists", func(t *testing.T) {
		db := setupProfileTestDB(t)
		service := &Service{DB: db}
		// First create a profile
		profile := createTestProfileData()
		// Set provider type before creating
		err := setProviderType(profile)
		assert.NoError(t, err)
		err = profile.Create(db)
		assert.NoError(t, err)

		// Try to validate a profile with the same ID
		newProfile := createTestProfileData()
		err = service.ValidateProfile(newProfile, 1, true)
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Bad Request", errResp.Title)
		assert.Contains(t, errResp.Message, "profile ID already exists")
	})

	t.Run("Invalid default user group ID", func(t *testing.T) {
		db := setupProfileTestDB(t)
		service := &Service{DB: db}
		profile := createTestProfileData()
		profile.DefaultUserGroupID = "invalid"
		err := service.ValidateProfile(profile, 1, true)
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Bad Request", errResp.Title)
		assert.Contains(t, errResp.Message, "invalid default user group ID")
	})

	t.Run("Default user group not found", func(t *testing.T) {
		db := setupProfileTestDB(t)
		service := &Service{DB: db}
		profile := createTestProfileData()
		profile.DefaultUserGroupID = "999"
		err := service.ValidateProfile(profile, 1, true)
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Not Found", errResp.Title)
		assert.Contains(t, errResp.Message, "default user group not found")
	})

	t.Run("Invalid user group ID in mapping", func(t *testing.T) {
		db := setupProfileTestDB(t)
		service := &Service{DB: db}
		profile := createTestProfileData()
		profile.UserGroupMapping = models.StringMap{"admin": "invalid"}
		err := service.ValidateProfile(profile, 1, true)
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Bad Request", errResp.Title)
		assert.Contains(t, errResp.Message, "invalid user group ID in mapping")
	})

	t.Run("User group in mapping not found", func(t *testing.T) {
		db := setupProfileTestDB(t)
		service := &Service{DB: db}
		profile := createTestProfileData()
		profile.UserGroupMapping = models.StringMap{"admin": "999"}
		err := service.ValidateProfile(profile, 1, true)
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Not Found", errResp.Title)
		assert.Contains(t, errResp.Message, "user group in mapping not found")
	})
}

func TestService_CreateProfile(t *testing.T) {
	db := setupProfileTestDB(t)
	service := &Service{DB: db}

	t.Run("Create valid profile", func(t *testing.T) {
		profile := createTestProfileData()
		err := service.CreateProfile(profile, 1)
		assert.NoError(t, err)
		assert.NotZero(t, profile.ID)

		// Verify profile was created
		fetchedProfile := models.NewProfile()
		err = fetchedProfile.Get(db, profile.ProfileID)
		assert.NoError(t, err)
		assert.Equal(t, profile.Name, fetchedProfile.Name)
		assert.Equal(t, profile.OrgID, fetchedProfile.OrgID)
		assert.Equal(t, profile.ProviderName, fetchedProfile.ProviderName)
		assert.Equal(t, provSocial, fetchedProfile.SelectedProviderType)
	})

	t.Run("Create invalid profile", func(t *testing.T) {
		profile := createTestProfileData()
		profile.ProfileID = ""
		profile.Name = ""
		err := service.CreateProfile(profile, 1)
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Bad Request", errResp.Title)
	})
}

func TestService_GetProfileByID(t *testing.T) {
	db := setupProfileTestDB(t)
	service := &Service{DB: db}

	// Create a profile first
	profile := createTestProfileData()
	// Set provider type before creating
	err := setProviderType(profile)
	assert.NoError(t, err)
	err = profile.Create(db)
	assert.NoError(t, err)

	t.Run("Get existing profile", func(t *testing.T) {
		fetchedProfile, err := service.GetProfileByID(profile.ProfileID)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedProfile)
		assert.Equal(t, profile.ProfileID, fetchedProfile.ProfileID)
		assert.Equal(t, profile.Name, fetchedProfile.Name)
	})

	t.Run("Get non-existent profile", func(t *testing.T) {
		fetchedProfile, err := service.GetProfileByID("non-existent-id")
		assert.Error(t, err)
		assert.Nil(t, fetchedProfile)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Not Found", errResp.Title)
	})
}

func TestService_UpdateProfile(t *testing.T) {
	db := setupProfileTestDB(t)
	service := &Service{DB: db}

	// Create a profile first
	profile := createTestProfileData()
	// Set provider type before creating
	err := setProviderType(profile)
	assert.NoError(t, err)
	err = profile.Create(db)
	assert.NoError(t, err)

	t.Run("Update profile without changing ID", func(t *testing.T) {
		updatedProfile := createTestProfileData()
		updatedProfile.Name = "Updated Profile Name"
		updatedProfile.OrgID = "updated-org-id"

		updatedProfile, err := service.UpdateProfile(profile.ProfileID, updatedProfile, 1)
		assert.NoError(t, err)
		assert.NotNil(t, updatedProfile)
		assert.Equal(t, "Updated Profile Name", updatedProfile.Name)
		assert.Equal(t, "updated-org-id", updatedProfile.OrgID)

		// Verify profile was updated in the database
		fetchedProfile, err := service.GetProfileByID(profile.ProfileID)
		assert.NoError(t, err)
		assert.Equal(t, "Updated Profile Name", fetchedProfile.Name)
		assert.Equal(t, "updated-org-id", fetchedProfile.OrgID)
	})

	t.Run("Update profile with changing ID", func(t *testing.T) {
		updatedProfile := createTestProfileData()
		updatedProfile.ProfileID = "new-profile-id"
		updatedProfile.Name = "New Profile Name"

		updatedProfile, err := service.UpdateProfile(profile.ProfileID, updatedProfile, 1)
		assert.NoError(t, err)
		assert.NotNil(t, updatedProfile)
		assert.Equal(t, "new-profile-id", updatedProfile.ProfileID)
		assert.Equal(t, "New Profile Name", updatedProfile.Name)

		// Verify profile was updated with new ID
		fetchedProfile, err := service.GetProfileByID("new-profile-id")
		assert.NoError(t, err)
		assert.Equal(t, "new-profile-id", fetchedProfile.ProfileID)
		assert.Equal(t, "New Profile Name", fetchedProfile.Name)

		// Old ID should no longer exist
		_, err = service.GetProfileByID(profile.ProfileID)
		assert.Error(t, err)
	})

	t.Run("Update non-existent profile", func(t *testing.T) {
		updatedProfile := createTestProfileData()
		_, err := service.UpdateProfile("non-existent-id", updatedProfile, 1)
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Not Found", errResp.Title)
	})

	t.Run("Update profile with invalid data", func(t *testing.T) {
		// Create a new profile for this test
		newProfile := createTestProfileData()
		newProfile.ProfileID = "another-profile-id"
		err := newProfile.Create(db)
		assert.NoError(t, err)

		updatedProfile := createTestProfileData()
		updatedProfile.DefaultUserGroupID = "invalid"

		_, err = service.UpdateProfile(newProfile.ProfileID, updatedProfile, 1)
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Bad Request", errResp.Title)
	})
}

func TestService_DeleteProfile(t *testing.T) {
	db := setupProfileTestDB(t)
	service := &Service{DB: db}

	// Create a profile first
	profile := createTestProfileData()
	// Set provider type before creating
	err := setProviderType(profile)
	assert.NoError(t, err)
	err = profile.Create(db)
	assert.NoError(t, err)

	t.Run("Delete existing profile", func(t *testing.T) {
		err := service.DeleteProfile(profile.ProfileID)
		assert.NoError(t, err)

		// Verify profile was deleted
		_, err = service.GetProfileByID(profile.ProfileID)
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Not Found", errResp.Title)
	})

	t.Run("Delete non-existent profile", func(t *testing.T) {
		err := service.DeleteProfile("non-existent-id")
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Not Found", errResp.Title)
	})
}

func TestService_ListProfiles(t *testing.T) {
	db := setupProfileTestDB(t)
	service := &Service{DB: db}

	// Create multiple profiles with different names and creation order
	profiles := []*models.Profile{
		{ProfileID: "profile-1", Name: "C Profile", OrgID: "org-1", DefaultUserGroupID: "1", ProviderConfig: models.JSONMap{"UseProviders": []interface{}{map[string]interface{}{"Name": "social"}}}},
		{ProfileID: "profile-2", Name: "A Profile", OrgID: "org-1", DefaultUserGroupID: "1", ProviderConfig: models.JSONMap{"UseProviders": []interface{}{map[string]interface{}{"Name": "social"}}}},
		{ProfileID: "profile-3", Name: "E Profile", OrgID: "org-2", DefaultUserGroupID: "1", ProviderConfig: models.JSONMap{"UseProviders": []interface{}{map[string]interface{}{"Name": "social"}}}},
		{ProfileID: "profile-4", Name: "B Profile", OrgID: "org-2", DefaultUserGroupID: "1", ProviderConfig: models.JSONMap{"UseProviders": []interface{}{map[string]interface{}{"Name": "social"}}}},
		{ProfileID: "profile-5", Name: "D Profile", OrgID: "org-3", DefaultUserGroupID: "1", ProviderConfig: models.JSONMap{"UseProviders": []interface{}{map[string]interface{}{"Name": "social"}}}},
	}

	for _, p := range profiles {
		err := setProviderType(p)
		assert.NoError(t, err)
		err = p.Create(db)
		assert.NoError(t, err)
	}

	t.Run("List profiles with pagination", func(t *testing.T) {
		fetchedProfiles, totalCount, totalPages, err := service.ListProfiles(2, 1, false, "")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages)
		assert.Len(t, fetchedProfiles, 2)
		// Default sort is by ID ascending, so first two profiles should be returned
		assert.Equal(t, uint(1), fetchedProfiles[0].ID)
		assert.Equal(t, uint(2), fetchedProfiles[1].ID)
	})

	t.Run("List profiles with different page", func(t *testing.T) {
		fetchedProfiles, totalCount, totalPages, err := service.ListProfiles(2, 2, false, "")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages)
		assert.Len(t, fetchedProfiles, 2)
		// Default sort is by ID ascending, so profiles 3 and 4 should be returned
		assert.Equal(t, uint(3), fetchedProfiles[0].ID)
		assert.Equal(t, uint(4), fetchedProfiles[1].ID)
	})

	t.Run("List all profiles", func(t *testing.T) {
		fetchedProfiles, totalCount, totalPages, err := service.ListProfiles(2, 1, true, "")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages)
		assert.Len(t, fetchedProfiles, 5)
		// Default sort is by ID ascending, so all profiles should be returned in ID order
		assert.Equal(t, uint(1), fetchedProfiles[0].ID)
		assert.Equal(t, uint(2), fetchedProfiles[1].ID)
		assert.Equal(t, uint(3), fetchedProfiles[2].ID)
		assert.Equal(t, uint(4), fetchedProfiles[3].ID)
		assert.Equal(t, uint(5), fetchedProfiles[4].ID)
	})

	t.Run("List profiles with sorting by name ascending", func(t *testing.T) {
		fetchedProfiles, totalCount, totalPages, err := service.ListProfiles(5, 1, true, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, fetchedProfiles, 5)
		// Profiles should be sorted by name in ascending order
		assert.Equal(t, "A Profile", fetchedProfiles[0].Name)
		assert.Equal(t, "B Profile", fetchedProfiles[1].Name)
		assert.Equal(t, "C Profile", fetchedProfiles[2].Name)
		assert.Equal(t, "D Profile", fetchedProfiles[3].Name)
		assert.Equal(t, "E Profile", fetchedProfiles[4].Name)
	})

	t.Run("List profiles with sorting by name descending", func(t *testing.T) {
		fetchedProfiles, totalCount, totalPages, err := service.ListProfiles(5, 1, true, "-name")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, fetchedProfiles, 5)
		// Profiles should be sorted by name in descending order
		assert.Equal(t, "E Profile", fetchedProfiles[0].Name)
		assert.Equal(t, "D Profile", fetchedProfiles[1].Name)
		assert.Equal(t, "C Profile", fetchedProfiles[2].Name)
		assert.Equal(t, "B Profile", fetchedProfiles[3].Name)
		assert.Equal(t, "A Profile", fetchedProfiles[4].Name)
	})

	t.Run("List profiles with sorting by org_id ascending", func(t *testing.T) {
		fetchedProfiles, totalCount, totalPages, err := service.ListProfiles(5, 1, true, "org_id")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, fetchedProfiles, 5)
		// First two profiles should have org_id "org-1"
		assert.Equal(t, "org-1", fetchedProfiles[0].OrgID)
		assert.Equal(t, "org-1", fetchedProfiles[1].OrgID)
		// Next two profiles should have org_id "org-2"
		assert.Equal(t, "org-2", fetchedProfiles[2].OrgID)
		assert.Equal(t, "org-2", fetchedProfiles[3].OrgID)
		// Last profile should have org_id "org-3"
		assert.Equal(t, "org-3", fetchedProfiles[4].OrgID)
	})

	t.Run("List profiles with sorting by org_id descending", func(t *testing.T) {
		fetchedProfiles, totalCount, totalPages, err := service.ListProfiles(5, 1, true, "-org_id")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Len(t, fetchedProfiles, 5)
		// First profile should have org_id "org-3"
		assert.Equal(t, "org-3", fetchedProfiles[0].OrgID)
		// Next two profiles should have org_id "org-2"
		assert.Equal(t, "org-2", fetchedProfiles[1].OrgID)
		assert.Equal(t, "org-2", fetchedProfiles[2].OrgID)
		// Last two profiles should have org_id "org-1"
		assert.Equal(t, "org-1", fetchedProfiles[3].OrgID)
		assert.Equal(t, "org-1", fetchedProfiles[4].OrgID)
	})

	t.Run("List profiles with pagination and sorting", func(t *testing.T) {
		fetchedProfiles, totalCount, totalPages, err := service.ListProfiles(2, 1, false, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages)
		assert.Len(t, fetchedProfiles, 2)
		// First two profiles by name should be returned
		assert.Equal(t, "A Profile", fetchedProfiles[0].Name)
		assert.Equal(t, "B Profile", fetchedProfiles[1].Name)
	})

	t.Run("List profiles with different page and sorting", func(t *testing.T) {
		fetchedProfiles, totalCount, totalPages, err := service.ListProfiles(2, 2, false, "name")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages)
		assert.Len(t, fetchedProfiles, 2)
		// Next two profiles by name should be returned
		assert.Equal(t, "C Profile", fetchedProfiles[0].Name)
		assert.Equal(t, "D Profile", fetchedProfiles[1].Name)
	})
}

func TestService_GetProfileByName(t *testing.T) {
	db := setupProfileTestDB(t)
	service := &Service{DB: db}

	// Create a profile first
	profile := createTestProfileData()
	// Set provider type before creating
	err := setProviderType(profile)
	assert.NoError(t, err)
	err = profile.Create(db)
	assert.NoError(t, err)

	t.Run("Get existing profile by name", func(t *testing.T) {
		fetchedProfile, err := service.GetProfileByName(profile.Name)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedProfile)
		assert.Equal(t, profile.ProfileID, fetchedProfile.ProfileID)
		assert.Equal(t, profile.Name, fetchedProfile.Name)
	})

	t.Run("Get non-existent profile by name", func(t *testing.T) {
		fetchedProfile, err := service.GetProfileByName("non-existent-name")
		assert.Error(t, err)
		assert.Nil(t, fetchedProfile)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Not Found", errResp.Title)
	})
}

func TestService_SetProfileUseInLoginPage(t *testing.T) {
	db := setupProfileTestDB(t)
	service := &Service{DB: db}

	// Create multiple profiles
	profile1 := createTestProfileData()
	profile1.UseInLoginPage = false
	err := setProviderType(profile1)
	assert.NoError(t, err)
	err = profile1.Create(db)
	assert.NoError(t, err)

	profile2 := createTestProfileData()
	profile2.ProfileID = "test-profile-id-2"
	profile2.UseInLoginPage = true // This one starts with UseInLoginPage=true
	err = setProviderType(profile2)
	assert.NoError(t, err)
	err = profile2.Create(db)
	assert.NoError(t, err)

	profile3 := createTestProfileData()
	profile3.ProfileID = "test-profile-id-3"
	profile3.UseInLoginPage = false
	err = setProviderType(profile3)
	assert.NoError(t, err)
	err = profile3.Create(db)
	assert.NoError(t, err)

	// Verify initial state
	var initialProfile models.Profile
	err = db.Where("profile_id = ?", profile2.ProfileID).First(&initialProfile).Error
	require.NoError(t, err)
	assert.True(t, initialProfile.UseInLoginPage, "Second profile should have UseInLoginPage=true initially")

	t.Run("Set profile use in login page", func(t *testing.T) {
		// Set profile1 to be used in login page
		err := service.SetProfileUseInLoginPage(profile1.ProfileID)
		assert.NoError(t, err)

		// Verify profile1 now has UseInLoginPage=true
		var updatedProfile1 models.Profile
		err = db.Where("profile_id = ?", profile1.ProfileID).First(&updatedProfile1).Error
		require.NoError(t, err)
		assert.True(t, updatedProfile1.UseInLoginPage, "First profile should have UseInLoginPage=true after update")

		// Verify profile2 now has UseInLoginPage=false
		var updatedProfile2 models.Profile
		err = db.Where("profile_id = ?", profile2.ProfileID).First(&updatedProfile2).Error
		require.NoError(t, err)
		assert.False(t, updatedProfile2.UseInLoginPage, "Second profile should have UseInLoginPage=false after update")

		// Verify profile3 still has UseInLoginPage=false
		var updatedProfile3 models.Profile
		err = db.Where("profile_id = ?", profile3.ProfileID).First(&updatedProfile3).Error
		require.NoError(t, err)
		assert.False(t, updatedProfile3.UseInLoginPage, "Third profile should have UseInLoginPage=false after update")
	})

	t.Run("Set profile use in login page for non-existent profile", func(t *testing.T) {
		err := service.SetProfileUseInLoginPage("non-existent-id")
		assert.Error(t, err)
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Not Found", errResp.Title)
	})
}

func TestService_GetLoginPageProfile(t *testing.T) {
	db := setupProfileTestDB(t)
	service := &Service{DB: db}

	t.Run("No profile set for login page", func(t *testing.T) {
		// When no profile is set for login page
		profile, err := service.GetLoginPageProfile()
		assert.Error(t, err)
		assert.Nil(t, profile)

		// Verify error type
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Not Found", errResp.Title)
		assert.Contains(t, errResp.Message, "no profile is set for use in login page")
	})

	t.Run("Profile set for login page", func(t *testing.T) {
		// Create a profile and set it for login page
		profile := createTestProfileData()
		err := profile.Create(db)
		require.NoError(t, err)

		// Set the profile for login page
		err = service.SetProfileUseInLoginPage(profile.ProfileID)
		require.NoError(t, err)

		// Get the login page profile
		fetchedProfile, err := service.GetLoginPageProfile()
		assert.NoError(t, err)
		assert.NotNil(t, fetchedProfile)
		assert.Equal(t, profile.ProfileID, fetchedProfile.ProfileID)
		assert.True(t, fetchedProfile.UseInLoginPage)
	})

	t.Run("Database error", func(t *testing.T) {
		// Create a new service with a closed DB connection to simulate a DB error
		closedDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		sqlDB, err := closedDB.DB()
		require.NoError(t, err)

		err = sqlDB.Close()
		require.NoError(t, err)

		errorService := &Service{DB: closedDB}

		// Attempt to get login page profile with a closed DB
		profile, err := errorService.GetLoginPageProfile()
		assert.Error(t, err)
		assert.Nil(t, profile)

		// Verify error type
		errResp, ok := err.(helpers.ErrorResponse)
		assert.True(t, ok)
		assert.Equal(t, "Internal Server Error", errResp.Title)
		assert.Contains(t, errResp.Message, "error getting login page profile")
	})
}
