package services

import (
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PendingAuthRequestService handles business logic for pending OAuth authorization requests.
type PendingAuthRequestService struct {
	db *gorm.DB
}

// NewPendingAuthRequestService creates a new PendingAuthRequestService.
func NewPendingAuthRequestService(db *gorm.DB) *PendingAuthRequestService {
	return &PendingAuthRequestService{db: db}
}

// StorePendingAuthRequestArgs holds arguments for storing a pending auth request.
type StorePendingAuthRequestArgs struct {
	ClientID            string
	UserID              uint
	RedirectURI         string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresIn           time.Duration // How long the pending request is valid
}

// StorePendingAuthRequest creates a new unique ID, stores the request details, and returns the ID.
func (s *PendingAuthRequestService) StorePendingAuthRequest(args StorePendingAuthRequestArgs) (*models.PendingOAuthRequest, error) {
	requestID := uuid.NewString()

	pendingRequest := &models.PendingOAuthRequest{
		ID:                  requestID,
		ClientID:            args.ClientID,
		UserID:              args.UserID,
		RedirectURI:         args.RedirectURI,
		Scope:               args.Scope,
		State:               args.State,
		CodeChallenge:       args.CodeChallenge,
		CodeChallengeMethod: args.CodeChallengeMethod,
		ExpiresAt:           time.Now().Add(args.ExpiresIn),
	}

	if err := s.db.Create(pendingRequest).Error; err != nil {
		return nil, err
	}
	return pendingRequest, nil
}

// GetPendingAuthRequest retrieves a pending auth request by its ID, checking for expiry.
// It also verifies that the provided userID matches the one in the stored request.
func (s *PendingAuthRequestService) GetPendingAuthRequest(id string, userID uint) (*models.PendingOAuthRequest, error) {
	var pendingRequest models.PendingOAuthRequest
	err := s.db.Where("id = ?", id).First(&pendingRequest).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("pending authorization request not found")
		}
		return nil, err
	}

	if time.Now().After(pendingRequest.ExpiresAt) {
		// Optionally, delete expired request here or have a separate cleanup job
		// s.DeletePendingAuthRequest(id)
		return nil, errors.New("pending authorization request has expired")
	}

	if pendingRequest.UserID != userID {
		// This is a critical security check to ensure the user fetching/acting on the consent request
		// is the same user who initiated it.
		return nil, errors.New("user mismatch for pending authorization request")
	}

	return &pendingRequest, nil
}

// DeletePendingAuthRequest removes a pending auth request from storage.
func (s *PendingAuthRequestService) DeletePendingAuthRequest(id string) error {
	// Using Unscoped().Delete to permanently delete, as these are temporary.
	// If soft delete is preferred, remove Unscoped().
	result := s.db.Unscoped().Where("id = ?", id).Delete(&models.PendingOAuthRequest{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("pending authorization request not found for deletion")
	}
	return nil
}
