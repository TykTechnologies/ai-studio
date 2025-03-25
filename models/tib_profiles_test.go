package models

import (
	"testing"

	"github.com/TykTechnologies/tyk-identity-broker/tap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupProfileTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate the Profile model
	err = db.AutoMigrate(&Profile{})
	require.NoError(t, err)

	return db
}

func createTestTIBProfile() *Profile {
	return &Profile{
		ProfileID:                 "test-profile-id",
		Name:                      "Test Profile",
		OrgID:                     "test-org-id",
		ActionType:                "GenerateOrLoginUserProfile",
		MatchedPolicyID:           "test-policy-id",
		Type:                      "social",
		ProviderName:              "social",
		CustomEmailField:          "email",
		CustomUserIDField:         "id",
		ProviderConfig:            JSONMap{"client_id": "test-client-id", "client_secret": "test-client-secret"},
		IdentityHandlerConfig:     JSONMap{"handler_type": "test-handler"},
		ProviderConstraintsDomain: "example.com",
		ProviderConstraintsGroup:  "test-group",
		ReturnURL:                 "https://example.com/return",
		DefaultUserGroupID:        "1",
		CustomUserGroupField:      "group",
		UserGroupMapping:          StringMap{"admin": "1", "user": "2"},
		UserGroupSeparator:        ",",
		SSOOnlyForRegisteredUsers: true,
	}
}

func TestProfile_NewProfile(t *testing.T) {
	profile := NewProfile()
	assert.NotNil(t, profile)
	assert.IsType(t, &Profile{}, profile)
}

func TestProfile_CRUD(t *testing.T) {
	db := setupProfileTestDB(t)
	profile := createTestTIBProfile()

	// Test Create
	err := profile.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, profile.ID)

	// Test Get
	fetchedProfile := NewProfile()
	err = fetchedProfile.Get(db, profile.ProfileID)
	assert.NoError(t, err)
	assert.Equal(t, profile.ProfileID, fetchedProfile.ProfileID)
	assert.Equal(t, profile.Name, fetchedProfile.Name)
	assert.Equal(t, profile.OrgID, fetchedProfile.OrgID)

	// Test Update
	profile.Name = "Updated Profile Name"
	err = profile.Update(db)
	assert.NoError(t, err)

	// Verify update
	updatedProfile := NewProfile()
	err = updatedProfile.Get(db, profile.ProfileID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated Profile Name", updatedProfile.Name)

	// Test Delete
	err = profile.Delete(db)
	assert.NoError(t, err)

	// Verify deletion
	deletedProfile := NewProfile()
	err = deletedProfile.Get(db, profile.ProfileID)
	assert.Error(t, err) // Should return error as profile is deleted
}

func TestProfile_GetByName(t *testing.T) {
	db := setupProfileTestDB(t)
	profile := createTestTIBProfile()

	// Create the profile
	err := profile.Create(db)
	assert.NoError(t, err)

	// Test GetByName
	fetchedProfile := NewProfile()
	err = fetchedProfile.GetByName(db, profile.Name)
	assert.NoError(t, err)
	assert.Equal(t, profile.ProfileID, fetchedProfile.ProfileID)
	assert.Equal(t, profile.Name, fetchedProfile.Name)

	// Test with non-existent name
	nonExistentProfile := NewProfile()
	err = nonExistentProfile.GetByName(db, "Non-existent Profile")
	assert.Error(t, err)
}

func TestProfiles_GetAll(t *testing.T) {
	db := setupProfileTestDB(t)

	// Create multiple profiles
	profiles := []*Profile{
		{ProfileID: "profile-1", Name: "Profile 1", OrgID: "org-1"},
		{ProfileID: "profile-2", Name: "Profile 2", OrgID: "org-1"},
		{ProfileID: "profile-3", Name: "Profile 3", OrgID: "org-2"},
		{ProfileID: "profile-4", Name: "Profile 4", OrgID: "org-2"},
		{ProfileID: "profile-5", Name: "Profile 5", OrgID: "org-3"},
	}

	for _, p := range profiles {
		err := p.Create(db)
		assert.NoError(t, err)
	}

	// Test GetAll with pagination
	var fetchedProfiles Profiles
	totalCount, totalPages, err := fetchedProfiles.GetAll(db, 2, 1, false)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), totalCount)
	assert.Equal(t, 3, totalPages)
	assert.Len(t, fetchedProfiles, 2)

	// Test GetAll with different page
	fetchedProfiles = Profiles{}
	totalCount, totalPages, err = fetchedProfiles.GetAll(db, 2, 2, false)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), totalCount)
	assert.Equal(t, 3, totalPages)
	assert.Len(t, fetchedProfiles, 2)

	// Test GetAll with all=true (no pagination)
	fetchedProfiles = Profiles{}
	totalCount, totalPages, err = fetchedProfiles.GetAll(db, 2, 1, true)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), totalCount)
	assert.Equal(t, 3, totalPages)
	assert.Len(t, fetchedProfiles, 5)
}

func TestProfile_MapToTapProfile(t *testing.T) {
	// Create a test profile
	profile := &Profile{
		ProfileID:                 "test-profile-id",
		Name:                      "Test Profile",
		OrgID:                     "test-org-id",
		ActionType:                "GenerateOrLoginUserProfile",
		MatchedPolicyID:           "test-policy-id",
		Type:                      "social",
		ProviderName:              "social",
		CustomEmailField:          "email",
		CustomUserIDField:         "id",
		ProviderConfig:            JSONMap{"client_id": "test-client-id", "client_secret": "test-client-secret"},
		IdentityHandlerConfig:     JSONMap{"handler_type": "test-handler"},
		ProviderConstraintsDomain: "example.com",
		ProviderConstraintsGroup:  "test-group",
		ReturnURL:                 "https://example.com/return",
		DefaultUserGroupID:        "1",
		CustomUserGroupField:      "group",
		UserGroupMapping:          StringMap{"admin": "1", "user": "2"},
		UserGroupSeparator:        ",",
		SSOOnlyForRegisteredUsers: true,
	}

	// Create a tap profile to map to
	tapProfile := &tap.Profile{}

	// Map the profile to the tap profile
	profile.MapToTapProfile(tapProfile)

	// Verify the mapping
	assert.Equal(t, profile.ProfileID, tapProfile.ID)
	assert.Equal(t, profile.Name, tapProfile.Name)
	assert.Equal(t, profile.OrgID, tapProfile.OrgID)
	assert.Equal(t, tap.Action(profile.ActionType), tapProfile.ActionType)
	assert.Equal(t, profile.MatchedPolicyID, tapProfile.MatchedPolicyID)
	assert.Equal(t, tap.ProviderType(profile.Type), tapProfile.Type)
	assert.Equal(t, profile.ProviderName, tapProfile.ProviderName)
	assert.Equal(t, profile.CustomEmailField, tapProfile.CustomEmailField)
	assert.Equal(t, profile.CustomUserIDField, tapProfile.CustomUserIDField)
	// Don't directly compare the ProviderConfig and IdentityHandlerConfig
	// as they may have different types (JSONMap vs map[string]interface{})
	assert.NotNil(t, tapProfile.ProviderConfig)
	assert.NotNil(t, tapProfile.IdentityHandlerConfig)
	assert.Equal(t, profile.ProviderConstraintsDomain, tapProfile.ProviderConstraints.Domain)
	assert.Equal(t, profile.ProviderConstraintsGroup, tapProfile.ProviderConstraints.Group)
	assert.Equal(t, profile.ReturnURL, tapProfile.ReturnURL)
	assert.Equal(t, profile.DefaultUserGroupID, tapProfile.DefaultUserGroupID)
	assert.Equal(t, profile.CustomUserGroupField, tapProfile.CustomUserGroupField)
	// Don't directly compare the UserGroupMapping
	// as they may have different types (StringMap vs map[string]string)
	assert.NotNil(t, tapProfile.UserGroupMapping)
	assert.Equal(t, profile.UserGroupSeparator, tapProfile.UserGroupSeparator)
	assert.Equal(t, profile.SSOOnlyForRegisteredUsers, tapProfile.SSOOnlyForRegisteredUsers)
}
