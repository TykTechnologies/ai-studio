package services_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apitesting "github.com/TykTechnologies/midsommar/v2/api/testing" // For SetupTestDB
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

func TestPendingAuthRequestService(t *testing.T) {
	db := apitesting.SetupTestDB(t)
	service := services.NewPendingAuthRequestService(db)

	// Create dummy users for association
	user1 := models.User{Email: "pendinguser1@example.com", Name: "Pending User 1"}
	require.NoError(t, db.Create(&user1).Error)
	user2 := models.User{Email: "pendinguser2@example.com", Name: "Pending User 2"}
	require.NoError(t, db.Create(&user2).Error)

	var createdRequest *models.PendingOAuthRequest

	storeArgs := services.StorePendingAuthRequestArgs{
		ClientID:            "pending-client-id",
		UserID:              user1.ID,
		RedirectURI:         "http://localhost/pending_callback",
		Scope:               "mcp offline_access",
		State:               "randomstate123",
		CodeChallenge:       "challenge_to_be_verified",
		CodeChallengeMethod: "S256",
		ExpiresIn:           5 * time.Minute,
	}

	t.Run("TestStorePendingAuthRequest", func(t *testing.T) {
		pendingRequest, err := service.StorePendingAuthRequest(storeArgs)
		require.NoError(t, err)
		require.NotNil(t, pendingRequest)
		createdRequest = pendingRequest

		require.NotEmpty(t, pendingRequest.ID)
		require.Equal(t, storeArgs.ClientID, pendingRequest.ClientID)
		require.Equal(t, storeArgs.UserID, pendingRequest.UserID)
		require.Equal(t, storeArgs.RedirectURI, pendingRequest.RedirectURI)
		require.Equal(t, storeArgs.Scope, pendingRequest.Scope)
		require.Equal(t, storeArgs.State, pendingRequest.State)
		require.Equal(t, storeArgs.CodeChallenge, pendingRequest.CodeChallenge)
		require.Equal(t, storeArgs.CodeChallengeMethod, pendingRequest.CodeChallengeMethod)
		require.WithinDuration(t, time.Now().Add(storeArgs.ExpiresIn), pendingRequest.ExpiresAt, time.Second)

		// Verify it's in DB
		var dbReq models.PendingOAuthRequest
		err = db.First(&dbReq, "id = ?", pendingRequest.ID).Error
		require.NoError(t, err)
		require.Equal(t, pendingRequest.ID, dbReq.ID)
	})

	t.Run("TestGetPendingAuthRequest_Valid", func(t *testing.T) {
		require.NotNil(t, createdRequest, "StorePendingAuthRequest must run first")

		retrievedReq, err := service.GetPendingAuthRequest(createdRequest.ID, user1.ID)
		require.NoError(t, err)
		require.NotNil(t, retrievedReq)
		require.Equal(t, createdRequest.ID, retrievedReq.ID)
		require.True(t, retrievedReq.ExpiresAt.After(time.Now()))
	})

	t.Run("TestGetPendingAuthRequest_NonExistent", func(t *testing.T) {
		_, err := service.GetPendingAuthRequest("nonexistent-req-id", user1.ID)
		require.Error(t, err)
		require.Equal(t, "pending authorization request not found", err.Error())
	})

	t.Run("TestGetPendingAuthRequest_UserMismatch", func(t *testing.T) {
		require.NotNil(t, createdRequest, "StorePendingAuthRequest must run first")
		_, err := service.GetPendingAuthRequest(createdRequest.ID, user2.ID) // Using different user ID
		require.Error(t, err)
		require.Equal(t, "user mismatch for pending authorization request", err.Error())
	})

	t.Run("TestGetPendingAuthRequest_Expired", func(t *testing.T) {
		expiredArgs := services.StorePendingAuthRequestArgs{
			ClientID: "exp-client", UserID: user1.ID, RedirectURI: "http://localhost/exp_callback",
			Scope: "test", ExpiresIn: -1 * time.Minute, // Already expired
			CodeChallenge: "exp_challenge", CodeChallengeMethod: "S256",
		}
		expiredReq, err := service.StorePendingAuthRequest(expiredArgs)
		require.NoError(t, err)

		_, err = service.GetPendingAuthRequest(expiredReq.ID, user1.ID)
		require.Error(t, err)
		require.Equal(t, "pending authorization request has expired", err.Error())
	})

	t.Run("TestDeletePendingAuthRequest", func(t *testing.T) {
		require.NotNil(t, createdRequest, "StorePendingAuthRequest must run first")

		// Store another one to ensure only one is deleted
		tempArgs := services.StorePendingAuthRequestArgs{ClientID: "temp-client", UserID: user1.ID, ExpiresIn: 5 * time.Minute}
		tempReq, err := service.StorePendingAuthRequest(tempArgs)
		require.NoError(t, err)

		// Delete the original createdRequest
		err = service.DeletePendingAuthRequest(createdRequest.ID)
		require.NoError(t, err)

		// Try to get the deleted one
		_, err = service.GetPendingAuthRequest(createdRequest.ID, user1.ID)
		require.Error(t, err)
		require.Equal(t, "pending authorization request not found", err.Error())

		// Ensure the temporary one still exists
		retrievedTempReq, err := service.GetPendingAuthRequest(tempReq.ID, user1.ID)
		require.NoError(t, err)
		require.NotNil(t, retrievedTempReq)

		// Clean up the temporary one
		err = service.DeletePendingAuthRequest(tempReq.ID)
		require.NoError(t, err)
	})

	t.Run("TestDeletePendingAuthRequest_NonExistent", func(t *testing.T) {
		err := service.DeletePendingAuthRequest("nonexistent-for-delete")
		require.Error(t, err)
		require.Equal(t, "pending authorization request not found for deletion", err.Error())
	})
}
