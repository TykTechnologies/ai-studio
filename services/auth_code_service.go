package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// AuthCodeService handles business logic for authorization codes.
type AuthCodeService struct {
	db *gorm.DB
}

// NewAuthCodeService creates a new AuthCodeService.
func NewAuthCodeService(db *gorm.DB) *AuthCodeService {
	return &AuthCodeService{db: db}
}

// GenerateSecureRandomString generates a secure random string of given length.
func GenerateSecureRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// CreateAuthCodeArgs holds arguments for creating an authorization code.
type CreateAuthCodeArgs struct {
	ClientID            string
	UserID              uint
	RedirectURI         string
	Scope               string
	ExpiresIn           time.Duration // How long the code is valid for
	CodeChallenge       string
	CodeChallengeMethod string
	AppID               *uint // Selected app ID for MCP OAuth
}

// CreateAuthCode generates, stores, and returns a new authorization code.
func (s *AuthCodeService) CreateAuthCode(args CreateAuthCodeArgs) (*models.AuthCode, string, error) {
	codeValue, err := GenerateSecureRandomString(32) // Generate a 64-character hex string
	if err != nil {
		return nil, "", err
	}

	authCode := &models.AuthCode{
		Code:                codeValue,
		ClientID:            args.ClientID,
		UserID:              args.UserID,
		RedirectURI:         args.RedirectURI,
		Scope:               args.Scope,
		ExpiresAt:           time.Now().Add(args.ExpiresIn),
		CodeChallenge:       args.CodeChallenge,
		CodeChallengeMethod: args.CodeChallengeMethod,
		Used:                false,
		AppID:               args.AppID,
	}

	if err := s.db.Create(authCode).Error; err != nil {
		return nil, "", err
	}
	return authCode, codeValue, nil
}

// GetValidAuthCodeByCode retrieves an authorization code by its value,
// checking for expiry and if it has already been used.
func (s *AuthCodeService) GetValidAuthCodeByCode(codeValue string) (*models.AuthCode, error) {
	var authCode models.AuthCode
	err := s.db.Where("code = ?", codeValue).First(&authCode).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("authorization code not found")
		}
		return nil, err
	}

	if authCode.Used {
		return nil, errors.New("authorization code has already been used")
	}

	if time.Now().After(authCode.ExpiresAt) {
		return nil, errors.New("authorization code has expired")
	}

	return &authCode, nil
}

// MarkAuthCodeAsUsed marks an authorization code as used.
func (s *AuthCodeService) MarkAuthCodeAsUsed(codeValue string) error {
	authCode, err := s.GetValidAuthCodeByCode(codeValue)
	if err != nil {
		// This check is important; ensure we only mark valid, unused, unexpired codes as used.
		// However, GetValidAuthCodeByCode already checks for used/expired.
		// If the intent is to mark any code as used regardless of its current state (e.g. for cleanup or specific revocation),
		// then a different retrieval method might be needed.
		// For the typical flow, this is correct: only a currently valid code can be marked used.
		return err
	}

	// Idempotency: if already marked used, GetValidAuthCodeByCode would have returned an error.
	// So, no need to check authCode.Used again here if GetValidAuthCodeByCode is strict.
	// However, if GetValidAuthCodeByCode changes, this explicit check might be useful.
	if authCode.Used {
		return errors.New("authorization code has already been used")
	}

	authCode.Used = true
	return s.db.Save(authCode).Error
}
