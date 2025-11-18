//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/sso"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createTestProfile(t *testing.T, db *gorm.DB) *models.Profile {
	// Create a test profile with all required fields
	profile := &models.Profile{
		Name:         "Test Profile",
		ProfileID:    "test-profile",
		ActionType:   "auth",
		Type:         "redirect",
		ProviderName: "SocialProvider",
		ProviderConfig: map[string]interface{}{
			"CallbackBaseURL": "http://localhost:8080/",
			"UseProviders": []map[string]interface{}{
				{
					"Name":   "social",
					"Key":    "test-key",
					"Secret": "test-secret",
				},
			},
		},
		IdentityHandlerConfig: map[string]interface{}{
			"DashboardCredential": "test-cred",
		},
		DefaultUserGroupID:   "1",
		SelectedProviderType: "social",
		UserID:               1,
	}

	err := db.Create(profile).Error
	require.NoError(t, err)
	return profile
}

func setupProfileTestService(t *testing.T) (*API, *gin.Engine, *gorm.DB) {
	// Use the existing setupTestAPI function which creates an in-memory database
	api, db := setupTestAPI(t)

	// Ensure tables are created
	err := db.AutoMigrate(&models.Profile{}, &models.User{}, &models.Group{})
	require.NoError(t, err)

	// Create default group
	defaultGroup := &models.Group{
		Name: "Default Group",
	}
	err = db.Create(defaultGroup).Error
	require.NoError(t, err)
	require.Equal(t, uint(1), defaultGroup.ID) // Default group should have ID 1

	// Create test user
	testUser := &models.User{
		Email:    "test@example.com",
		Name:     "Test User",
		Password: "password",
		IsAdmin:  true,
	}
	err = db.Create(testUser).Error
	require.NoError(t, err)

	// Setup config and SSO service
	authConfig := &auth.Config{
		DB:                  db,
		Service:             api.service,
		CookieName:          "session",
		CookieSecure:        true,
		CookieHTTPOnly:      true,
		CookieSameSite:      http.SameSiteStrictMode,
		ResetTokenExpiry:    3600,
		FrontendURL:         "http://example.com",
		RegistrationAllowed: true,
		AdminEmail:          "admin@example.com",
		TestMode:            true,
	}

	ssoConfig := &sso.Config{
		APISecret: "test-secret",
		LogLevel:  "info",
	}

	ssoService := sso.NewService(ssoConfig, gin.New(), db, nil)
	if err := ssoService.InitInternalTIB(); err != nil {
		t.Fatalf("Failed to initialize SSO service: %v", err)
	}
	api.ssoService = ssoService
	api.config = authConfig
	api.config.TIBAPISecret = ssoConfig.APISecret
	api.config.TIBEnabled = true // Enable TIB for tests

	// Setup router
	r := gin.New()

	// Add authentication middleware that adds the test user to the context
	r.Use(func(c *gin.Context) {
		c.Set("user", testUser)
		c.Next()
	})

	return api, r, db
}

func TestCreateProfile(t *testing.T) {
	api, r, _ := setupProfileTestService(t)
	r.POST("/api/v1/sso-profiles", api.createProfile)

	t.Run("Valid profile creation", func(t *testing.T) {
		// Prepare valid profile input
		input := map[string]interface{}{
			"data": map[string]interface{}{
				"type": "sso-profiles",
				"attributes": map[string]interface{}{
					"name":                 "Test Profile",
					"org_id":               "",
					"action_type":          "auth",
					"matched_policy_id":    "",
					"type":                 "redirect",
					"provider_name":        "SocialProvider",
					"custom_email_field":   "",
					"custom_user_id_field": "",
					"provider_config": map[string]interface{}{
						"CallbackBaseURL": "http://localhost:8080/",
						"UseProviders": []map[string]interface{}{
							{
								"Name":   "social",
								"Key":    "test-key",
								"Secret": "test-secret",
							},
						},
					},
					"identity_handler_config":       map[string]interface{}{},
					"provider_constraints_domain":   "",
					"provider_constraints_group":    "",
					"return_url":                    "",
					"default_user_group_id":         "1",
					"custom_user_group_field":       "",
					"user_group_mapping":            map[string]string{},
					"user_group_separator":          "",
					"sso_only_for_registered_users": false,
				},
			},
		}

		w := performRequest(r, "POST", "/api/v1/sso-profiles", input)
		assert.Equal(t, http.StatusCreated, w.Code)

		// Only parse the response if the status code is as expected
		if w.Code == http.StatusCreated {
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			profileData, ok := response["data"].(map[string]interface{})
			require.True(t, ok, "Response data is not a map")
			require.NotNil(t, profileData, "Profile data is nil")

			profileAttrs := profileData["attributes"].(map[string]interface{})
			assert.Equal(t, "sso-profiles", profileData["type"])
			assert.Equal(t, "Test Profile", profileAttrs["name"])
			assert.Equal(t, "auth", profileAttrs["action_type"])
			assert.Equal(t, "redirect", profileAttrs["type"])
			assert.Equal(t, "SocialProvider", profileAttrs["provider_name"])
			assert.Equal(t, "1", profileAttrs["default_user_group_id"])
			assert.Equal(t, "social", profileAttrs["selected_provider_type"])
			assert.NotEmpty(t, profileAttrs["login_url"])
			assert.NotEmpty(t, profileAttrs["callback_url"])
		}
	})

	t.Run("Invalid request body", func(t *testing.T) {
		w := performRequest(r, "POST", "/api/v1/sso-profiles", "invalid json")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Missing required fields", func(t *testing.T) {
		// Prepare invalid profile input (missing required fields)
		input := map[string]interface{}{
			"data": map[string]interface{}{
				"type": "sso-profiles",
				"attributes": map[string]interface{}{
					"name": "Test Profile",
					// Missing ActionType, Type, and ProviderName
				},
			},
		}

		w := performRequest(r, "POST", "/api/v1/sso-profiles", input)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestGetProfile(t *testing.T) {
	api, r, db := setupProfileTestService(t)
	r.GET("/api/v1/sso-profiles/:profile_id", api.getProfile)

	profile := createTestProfile(t, db)

	t.Run("Get existing profile", func(t *testing.T) {
		w := performRequest(r, "GET", "/api/v1/sso-profiles/"+profile.ProfileID, nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		profileData := response["data"].(map[string]interface{})
		profileAttrs := profileData["attributes"].(map[string]interface{})
		assert.Equal(t, "sso-profiles", profileData["type"])
		assert.Equal(t, float64(profile.Model.ID), profileData["id"])
		assert.Equal(t, profile.Name, profileAttrs["name"])
		assert.Equal(t, profile.ActionType, profileAttrs["action_type"])
		assert.Equal(t, profile.Type, profileAttrs["type"])
		assert.Equal(t, profile.ProviderName, profileAttrs["provider_name"])
	})

	t.Run("Get non-existent profile", func(t *testing.T) {
		w := performRequest(r, "GET", "/api/v1/sso-profiles/non-existent", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestUpdateProfile(t *testing.T) {
	api, r, db := setupProfileTestService(t)
	r.PUT("/api/v1/sso-profiles/:profile_id", api.updateProfile)

	// Create a test profile directly
	profile := &models.Profile{
		Name:         "Test Profile",
		ProfileID:    "test-profile",
		ActionType:   "auth",
		Type:         "redirect",
		ProviderName: "SocialProvider",
		ProviderConfig: map[string]interface{}{
			"CallbackBaseURL": "http://localhost:8080/",
			"UseProviders": []map[string]interface{}{
				{
					"Name":   "social",
					"Key":    "test-key",
					"Secret": "test-secret",
				},
			},
		},
		IdentityHandlerConfig: map[string]interface{}{
			"DashboardCredential": "test-cred",
		},
		DefaultUserGroupID:   "1",
		SelectedProviderType: "social",
		UserID:               1,
	}

	err := db.Create(profile).Error
	require.NoError(t, err)

	// Verify the profile was created
	var count int64
	err = db.Model(&models.Profile{}).Where("profile_id = ?", profile.ProfileID).Count(&count).Error
	require.NoError(t, err)
	require.Equal(t, int64(1), count, "Profile should exist in the database")

	t.Run("Update existing profile", func(t *testing.T) {
		// Prepare update input with complete provider configuration
		input := map[string]interface{}{
			"data": map[string]interface{}{
				"type": "sso-profiles",
				"id":   profile.ProfileID,
				"attributes": map[string]interface{}{
					"name":          "Updated Profile",
					"action_type":   "auth",
					"type":          "redirect",
					"provider_name": "SocialProvider",
					"provider_config": map[string]interface{}{
						"CallbackBaseURL": "http://localhost:8080/",
						"UseProviders": []map[string]interface{}{
							{
								"Name":   "social",
								"Key":    "test-key",
								"Secret": "test-secret",
							},
						},
					},
					"identity_handler_config": map[string]interface{}{
						"DashboardCredential": "test-cred",
					},
					"default_user_group_id": "1",
				},
			},
		}
		w := performRequest(r, "PUT", "/api/v1/sso-profiles/"+profile.ProfileID, input)
		assert.Equal(t, http.StatusOK, w.Code)

		// Print the response body for debugging
		fmt.Printf("Response body: %s\n", w.Body.String())

		// Only parse the response if the status code is as expected
		if w.Code == http.StatusOK {
			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			profileData, ok := response["data"].(map[string]interface{})
			require.True(t, ok, "Response data is not a map")
			require.NotNil(t, profileData, "Profile data is nil")

			profileAttrs := profileData["attributes"].(map[string]interface{})
			assert.Equal(t, "sso-profiles", profileData["type"])
			// The ID field is the GORM Model ID, not the ProfileID
			assert.Equal(t, float64(profile.Model.ID), profileData["id"])
			assert.Equal(t, "Updated Profile", profileAttrs["name"])
			assert.Equal(t, "social", profileAttrs["selected_provider_type"])
		}
	})

	t.Run("Update non-existent profile", func(t *testing.T) {
		input := map[string]interface{}{
			"data": map[string]interface{}{
				"type": "sso-profiles",
				"id":   "non-existent",
				"attributes": map[string]interface{}{
					"name":          "Updated Profile",
					"action_type":   "auth",
					"type":          "redirect",
					"provider_name": "SocialProvider",
					"provider_config": map[string]interface{}{
						"CallbackBaseURL": "http://localhost:8080/",
						"UseProviders": []map[string]interface{}{
							{
								"Name":   "social",
								"Key":    "test-key",
								"Secret": "test-secret",
							},
						},
					},
					"identity_handler_config": map[string]interface{}{
						"DashboardCredential": "test-cred",
					},
					"default_user_group_id": "1",
				},
			},
		}

		w := performRequest(r, "PUT", "/api/v1/sso-profiles/non-existent", input)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Invalid request body", func(t *testing.T) {
		w := performRequest(r, "PUT", "/api/v1/sso-profiles/"+profile.ProfileID, "invalid json")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestDeleteProfile(t *testing.T) {
	api, r, db := setupProfileTestService(t)
	r.DELETE("/api/v1/sso-profiles/:profile_id", api.deleteProfile)

	// Create a test profile using the helper function
	profile := createTestProfile(t, db)

	t.Run("Delete existing profile", func(t *testing.T) {
		w := performRequest(r, "DELETE", "/api/v1/sso-profiles/"+profile.ProfileID, nil)
		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify profile is deleted
		var count int64
		db.Model(&models.Profile{}).Where("profile_id = ?", profile.ProfileID).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("Delete non-existent profile", func(t *testing.T) {
		w := performRequest(r, "DELETE", "/api/v1/sso-profiles/non-existent", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestListProfiles(t *testing.T) {
	api, r, db := setupProfileTestService(t)
	r.GET("/api/v1/sso-profiles", api.listProfiles)

	// Create test profiles using the helper function
	profile1 := createTestProfile(t, db)

	// Create a second profile with a different name and ID
	profile2 := &models.Profile{
		Name:         "Profile 2",
		ProfileID:    "profile-2",
		ActionType:   "auth",
		Type:         "redirect",
		ProviderName: "SocialProvider",
		ProviderConfig: map[string]interface{}{
			"CallbackBaseURL": "http://localhost:8080/",
			"UseProviders": []map[string]interface{}{
				{
					"Name":   "social",
					"Key":    "test-key",
					"Secret": "test-secret",
				},
			},
		},
		IdentityHandlerConfig: map[string]interface{}{
			"DashboardCredential": "test-cred",
		},
		DefaultUserGroupID:   "1",
		SelectedProviderType: "social",
		UserID:               1,
	}
	err := db.Create(profile2).Error
	require.NoError(t, err)

	// Store profiles for later assertion
	profiles := []*models.Profile{profile1, profile2}

	t.Run("List all profiles", func(t *testing.T) {
		w := performRequest(r, "GET", "/api/v1/sso-profiles", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		assert.Equal(t, len(profiles), len(data))
	})

	t.Run("List profiles with sorting", func(t *testing.T) {
		w := performRequest(r, "GET", "/api/v1/sso-profiles?sort=name", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		assert.Equal(t, len(profiles), len(data))
	})

	t.Run("List profiles with descending sorting", func(t *testing.T) {
		w := performRequest(r, "GET", "/api/v1/sso-profiles?sort=-name", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		data := response["data"].([]interface{})
		assert.Equal(t, len(profiles), len(data))
	})
}

// TestSerializeProfile tests the serializeProfile function with different provider types
// and URL formatting scenarios
func TestSerializeProfile(t *testing.T) {
	t.Run("Social provider", func(t *testing.T) {
		profile := &models.Profile{
			Name:         "Social Profile",
			ProfileID:    "social-profile",
			ActionType:   "auth",
			Type:         "redirect",
			ProviderName: "SocialProvider",
			ProviderConfig: map[string]interface{}{
				"CallbackBaseURL": "http://localhost:8080/",
				"UseProviders": []map[string]interface{}{
					{
						"Name":   "social",
						"Key":    "test-key",
						"Secret": "test-secret",
					},
				},
			},
			SelectedProviderType: "social",
		}

		response := serializeProfile(profile)

		// Check that URLs are correctly formatted with trailing slash
		assert.Equal(t, "http://localhost:8080/auth/social-profile/social", response.Attributes.LoginURL)
		assert.Equal(t, "http://localhost:8080/auth/social-profile/social/callback", response.Attributes.CallbackURL)
	})

	t.Run("Social provider without trailing slash in CallbackBaseURL", func(t *testing.T) {
		profile := &models.Profile{
			Name:         "Social Profile No Slash",
			ProfileID:    "social-profile-no-slash",
			ActionType:   "auth",
			Type:         "redirect",
			ProviderName: "SocialProvider",
			ProviderConfig: map[string]interface{}{
				"CallbackBaseURL": "http://localhost:8080", // No trailing slash
				"UseProviders": []map[string]interface{}{
					{
						"Name":   "social",
						"Key":    "test-key",
						"Secret": "test-secret",
					},
				},
			},
			SelectedProviderType: "social",
		}

		response := serializeProfile(profile)

		// Check that URLs are correctly formatted even without trailing slash in CallbackBaseURL
		assert.Equal(t, "http://localhost:8080/auth/social-profile-no-slash/social", response.Attributes.LoginURL)
		assert.Equal(t, "http://localhost:8080/auth/social-profile-no-slash/social/callback", response.Attributes.CallbackURL)
	})

	t.Run("SAML provider with SAMLBaseURL", func(t *testing.T) {
		profile := &models.Profile{
			Name:         "SAML Profile",
			ProfileID:    "saml-profile",
			ActionType:   "auth",
			Type:         "redirect",
			ProviderName: "SAMLProvider",
			ProviderConfig: map[string]interface{}{
				"CallbackBaseURL": "http://localhost:8080/",
				"SAMLBaseURL":     "https://saml.example.com/",
			},
			SelectedProviderType: "saml",
		}

		response := serializeProfile(profile)

		// Check that URLs use SAMLBaseURL instead of CallbackBaseURL
		assert.Equal(t, "https://saml.example.com/auth/saml-profile/saml", response.Attributes.LoginURL)
		assert.Equal(t, "https://saml.example.com/auth/saml-profile/saml/callback", response.Attributes.CallbackURL)
	})

	t.Run("SAML provider with SAMLBaseURL without trailing slash", func(t *testing.T) {
		profile := &models.Profile{
			Name:         "SAML Profile No Slash",
			ProfileID:    "saml-profile-no-slash",
			ActionType:   "auth",
			Type:         "redirect",
			ProviderName: "SAMLProvider",
			ProviderConfig: map[string]interface{}{
				"CallbackBaseURL": "http://localhost:8080/",
				"SAMLBaseURL":     "https://saml.example.com", // No trailing slash
			},
			SelectedProviderType: "saml",
		}

		response := serializeProfile(profile)

		// Check that URLs are correctly formatted even without trailing slash in SAMLBaseURL
		assert.Equal(t, "https://saml.example.com/auth/saml-profile-no-slash/saml", response.Attributes.LoginURL)
		assert.Equal(t, "https://saml.example.com/auth/saml-profile-no-slash/saml/callback", response.Attributes.CallbackURL)
	})
}

// TestSerializeProfileList tests the serializeProfileList function with different provider types
func TestSerializeProfileList(t *testing.T) {
	t.Run("Social provider", func(t *testing.T) {
		profile := &models.Profile{
			Name:                 "Social Profile",
			ProfileID:            "social-profile",
			SelectedProviderType: "social",
			ActionType:           "GenerateOrLoginUserProfile",
		}

		response := serializeProfileList(profile)

		// Check that provider type is correctly mapped
		assert.Equal(t, "Social", response.Attributes.ProviderType)
		assert.Equal(t, userProfile, response.Attributes.ProfileType)
	})

	t.Run("OpenID Connect provider", func(t *testing.T) {
		profile := &models.Profile{
			Name:                 "OpenID Profile",
			ProfileID:            "openid-profile",
			SelectedProviderType: provOpenID,
			ActionType:           "GenerateOrLoginUserProfile",
		}

		response := serializeProfileList(profile)

		// Check that provider type is correctly mapped
		assert.Equal(t, "Open ID Connect", response.Attributes.ProviderType)
		assert.Equal(t, userProfile, response.Attributes.ProfileType)
	})

	t.Run("LDAP provider", func(t *testing.T) {
		profile := &models.Profile{
			Name:                 "LDAP Profile",
			ProfileID:            "ldap-profile",
			SelectedProviderType: provLDAP,
			ActionType:           "GenerateOrLoginUserProfile",
		}

		response := serializeProfileList(profile)

		// Check that provider type is correctly mapped
		assert.Equal(t, "LDAP", response.Attributes.ProviderType)
		assert.Equal(t, userProfile, response.Attributes.ProfileType)
	})

	t.Run("SAML provider", func(t *testing.T) {
		profile := &models.Profile{
			Name:                 "SAML Profile",
			ProfileID:            "saml-profile",
			SelectedProviderType: provSAML,
			ActionType:           "GenerateOrLoginUserProfile",
		}

		response := serializeProfileList(profile)

		// Check that provider type is correctly mapped
		assert.Equal(t, "SAML", response.Attributes.ProviderType)
		assert.Equal(t, userProfile, response.Attributes.ProfileType)
	})
}

func TestSetProfileUseInLoginPage(t *testing.T) {
	api, r, db := setupProfileTestService(t)
	r.POST("/api/v1/sso-profiles/:profile_id/use-in-login-page", api.setProfileUseInLoginPage)

	// Create multiple test profiles
	profile1 := createTestProfile(t, db)
	profile1.UseInLoginPage = false
	err := db.Save(profile1).Error
	require.NoError(t, err)

	profile2 := createTestProfile(t, db)
	profile2.ProfileID = "test-profile-2"
	profile2.UseInLoginPage = true // This one starts with UseInLoginPage=true
	err = db.Save(profile2).Error
	require.NoError(t, err)

	t.Run("Set profile use in login page", func(t *testing.T) {
		w := performRequest(r, "POST", "/api/v1/sso-profiles/"+profile1.ProfileID+"/use-in-login-page", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		// Verify profile1 now has UseInLoginPage=true
		var updatedProfile1 models.Profile
		err := db.Where("profile_id = ?", profile1.ProfileID).First(&updatedProfile1).Error
		require.NoError(t, err)
		assert.True(t, updatedProfile1.UseInLoginPage, "First profile should have UseInLoginPage=true after update")

		// Verify profile2 now has UseInLoginPage=false
		var updatedProfile2 models.Profile
		err = db.Where("profile_id = ?", profile2.ProfileID).First(&updatedProfile2).Error
		require.NoError(t, err)
		assert.False(t, updatedProfile2.UseInLoginPage, "Second profile should have UseInLoginPage=false after update")
	})

	t.Run("Set profile use in login page for non-existent profile", func(t *testing.T) {
		w := performRequest(r, "POST", "/api/v1/sso-profiles/non-existent/use-in-login-page", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		errors := response["errors"].([]interface{})
		assert.NotEmpty(t, errors)

		errorObj := errors[0].(map[string]interface{})
		assert.Equal(t, "Not Found", errorObj["title"])
	})

	t.Run("Invalid profile ID", func(t *testing.T) {
		w := performRequest(r, "POST", "/api/v1/sso-profiles//use-in-login-page", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		errors := response["errors"].([]interface{})
		assert.NotEmpty(t, errors)

		errorObj := errors[0].(map[string]interface{})
		assert.Equal(t, "Bad Request", errorObj["title"])
		assert.Contains(t, errorObj["detail"], "Invalid profile ID")
	})
}

func TestGetLoginPageProfile(t *testing.T) {
	api, r, db := setupProfileTestService(t)
	r.GET("/api/v1/sso-profiles/login-page", api.getLoginPageProfile)

	t.Run("No profile set for login page", func(t *testing.T) {
		// When no profile is set for login page
		w := performRequest(r, "GET", "/api/v1/sso-profiles/login-page", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)

		// Check that we get a 404 status code
		assert.Equal(t, http.StatusNotFound, w.Code)

		// Parse the error response
		var errorResponse struct {
			Errors []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			} `json:"errors"`
		}

		err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
		require.NoError(t, err)

		require.Len(t, errorResponse.Errors, 1, "Expected one error in the response")
		assert.Equal(t, "Not Found", errorResponse.Errors[0].Title)
		assert.Contains(t, errorResponse.Errors[0].Detail, "no profile is set for use in login page")
		// We'll update the assertions once we see the actual response structure
	})

	t.Run("Profile set for login page", func(t *testing.T) {
		// Create a profile and set it for login page
		profile := createTestProfile(t, db)

		// Set the profile for login page
		err := db.Model(&models.Profile{}).Where("profile_id = ?", profile.ProfileID).Update("use_in_login_page", true).Error
		require.NoError(t, err)

		// Get the login page profile
		w := performRequest(r, "GET", "/api/v1/sso-profiles/login-page", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		profileData := response["data"].(map[string]interface{})
		profileAttrs := profileData["attributes"].(map[string]interface{})
		assert.Equal(t, "sso-profiles", profileData["type"])
		assert.Equal(t, float64(profile.Model.ID), profileData["id"])
		assert.Equal(t, profile.Name, profileAttrs["name"])
		assert.Equal(t, profile.ProfileID, profileAttrs["profile_id"])
		assert.Equal(t, true, profileAttrs["use_in_login_page"])
	})
}
