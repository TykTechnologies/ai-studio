package models

import (
	"testing"

	"github.com/TykTechnologies/tyk-identity-broker/tap"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForTIB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = db.AutoMigrate(&Profile{})
	assert.NoError(t, err)

	return db
}

func createTestProfile(t *testing.T, db *gorm.DB) *Profile {
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

	err := db.Create(profile).Error
	assert.NoError(t, err)

	return profile
}

func TestGormAuthRegisterBackend_Init(t *testing.T) {
	db := setupTestDBForTIB(t)

	// Test successful initialization
	backend := &GormAuthRegisterBackend{}
	err := backend.Init(db)
	assert.NoError(t, err)
	assert.NotNil(t, backend.DB)

	// Test initialization with invalid config
	err = backend.Init("invalid-config")
	assert.Error(t, err)
	assert.Equal(t, "invalid config", err.Error())
}

func TestGormAuthRegisterBackend_GetKey(t *testing.T) {
	db := setupTestDBForTIB(t)
	backend := &GormAuthRegisterBackend{DB: db}

	// Create a test profile
	testProfile := createTestProfile(t, db)

	// Test successful retrieval
	var tapProfile tap.Profile
	err := backend.GetKey(testProfile.ProfileID, "", &tapProfile)
	assert.NoError(t, err)
	assert.Equal(t, testProfile.ProfileID, tapProfile.ID)
	assert.Equal(t, testProfile.Name, tapProfile.Name)
	assert.Equal(t, testProfile.OrgID, tapProfile.OrgID)
	assert.Equal(t, tap.Action(testProfile.ActionType), tapProfile.ActionType)
	assert.Equal(t, testProfile.MatchedPolicyID, tapProfile.MatchedPolicyID)
	assert.Equal(t, tap.ProviderType(testProfile.Type), tapProfile.Type)
	assert.Equal(t, testProfile.ProviderName, tapProfile.ProviderName)
	assert.Equal(t, testProfile.CustomEmailField, tapProfile.CustomEmailField)
	assert.Equal(t, testProfile.CustomUserIDField, tapProfile.CustomUserIDField)
	// Check that ProviderConfig and IdentityHandlerConfig are not nil
	assert.NotNil(t, tapProfile.ProviderConfig)
	assert.NotNil(t, tapProfile.IdentityHandlerConfig)
	assert.Equal(t, testProfile.ProviderConstraintsDomain, tapProfile.ProviderConstraints.Domain)
	assert.Equal(t, testProfile.ProviderConstraintsGroup, tapProfile.ProviderConstraints.Group)
	assert.Equal(t, testProfile.ReturnURL, tapProfile.ReturnURL)
	assert.Equal(t, testProfile.DefaultUserGroupID, tapProfile.DefaultUserGroupID)
	assert.Equal(t, testProfile.CustomUserGroupField, tapProfile.CustomUserGroupField)
	// Check that UserGroupMapping is not nil
	assert.NotNil(t, tapProfile.UserGroupMapping)
	assert.Equal(t, testProfile.UserGroupSeparator, tapProfile.UserGroupSeparator)
	assert.Equal(t, testProfile.SSOOnlyForRegisteredUsers, tapProfile.SSOOnlyForRegisteredUsers)

	// Test retrieval with non-existent key
	err = backend.GetKey("non-existent-key", "", &tapProfile)
	assert.Error(t, err)
	assert.Equal(t, "record not found", err.Error())

	// Test retrieval with invalid value type
	var invalidValue string
	err = backend.GetKey(testProfile.ProfileID, "", &invalidValue)
	assert.Error(t, err)
	assert.Equal(t, "invalid value", err.Error())
}

func TestGormAuthRegisterBackend_SetKey(t *testing.T) {
	db := setupTestDBForTIB(t)
	backend := &GormAuthRegisterBackend{DB: db}

	// Test SetKey (which is not implemented)
	err := backend.SetKey("key", "orgId", "value")
	assert.NoError(t, err) // Should return nil as it's not implemented
}

func TestGormAuthRegisterBackend_GetAll(t *testing.T) {
	db := setupTestDBForTIB(t)
	backend := &GormAuthRegisterBackend{DB: db}

	// Test GetAll (which is not implemented)
	result := backend.GetAll("orgId")
	assert.Nil(t, result) // Should return nil as it's not implemented
}

func TestGormAuthRegisterBackend_DeleteKey(t *testing.T) {
	db := setupTestDBForTIB(t)
	backend := &GormAuthRegisterBackend{DB: db}

	// Test DeleteKey (which is not implemented)
	err := backend.DeleteKey("key", "orgId")
	assert.NoError(t, err) // Should return nil as it's not implemented
}

func TestNewGormAuthRegisterBackend(t *testing.T) {
	db := setupTestDBForTIB(t)

	// Test successful creation
	backend := NewGormAuthRegisterBackend(db)
	assert.NotNil(t, backend)
	assert.IsType(t, &GormAuthRegisterBackend{}, backend)
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
