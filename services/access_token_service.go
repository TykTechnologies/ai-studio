package services

import (
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// AccessTokenService handles business logic for access tokens.
type AccessTokenService struct {
	db *gorm.DB
}

// NewAccessTokenService creates a new AccessTokenService.
func NewAccessTokenService(db *gorm.DB) *AccessTokenService {
	return &AccessTokenService{db: db}
}

// CreateAccessTokenArgs holds arguments for creating an access token.
type CreateAccessTokenArgs struct {
	ClientID  string
	UserID    uint
	Scope     string
	ExpiresIn time.Duration // How long the token is valid for
}

// CreateAccessToken generates, stores, and returns a new access token.
// The actual token value is also returned for immediate use.
func (s *AccessTokenService) CreateAccessToken(args CreateAccessTokenArgs) (*models.AccessToken, string, error) {
	// Generate a secure random string for the token value.
	// Length can be adjusted; e.g., 32 bytes for a 64-character hex string.
	tokenValue, err := GenerateSecureRandomString(32)
	if err != nil {
		return nil, "", err
	}

	accessToken := &models.AccessToken{
		Token:     tokenValue,
		ClientID:  args.ClientID,
		UserID:    args.UserID,
		Scope:     args.Scope,
		ExpiresAt: time.Now().Add(args.ExpiresIn),
	}

	if err := s.db.Create(accessToken).Error; err != nil {
		return nil, "", err
	}
	return accessToken, tokenValue, nil
}

// GetValidAccessTokenByToken retrieves an access token by its value and checks if it's valid (not expired).
func (s *AccessTokenService) GetValidAccessTokenByToken(tokenValue string) (*models.AccessToken, error) {
	var accessToken models.AccessToken
	err := s.db.Where("token = ?", tokenValue).First(&accessToken).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("access token not found")
		}
		return nil, err
	}

	if time.Now().After(accessToken.ExpiresAt) {
		return nil, errors.New("access token has expired")
	}

	return &accessToken, nil
}
