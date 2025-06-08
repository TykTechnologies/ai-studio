package services

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"errors"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// OAuthClientService handles business logic for OAuth clients.
type OAuthClientService struct {
	db *gorm.DB
}

// NewOAuthClientService creates a new OAuthClientService.
func NewOAuthClientService(db *gorm.DB) *OAuthClientService {
	return &OAuthClientService{db: db}
}

// CreateClient creates a new OAuth client.
// It returns the client object and the plain text client secret (which should be shown to the user once).
func (s *OAuthClientService) CreateClient(name string, redirectURIs []string, userID uint, scope string) (*models.OAuthClient, string, error) {
	// Generate unique ClientID
	rawClientID := make([]byte, 16) // 128-bit
	if _, err := rand.Read(rawClientID); err != nil {
		return nil, "", err
	}
	clientID := hex.EncodeToString(rawClientID)

	// Generate ClientSecret
	rawClientSecret := make([]byte, 32) // 256-bit
	if _, err := rand.Read(rawClientSecret); err != nil {
		return nil, "", err
	}
	plainClientSecret := hex.EncodeToString(rawClientSecret)

	// Hash the ClientSecret
	hashedSecret, err := bcrypt.GenerateFromPassword([]byte(plainClientSecret), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}

	client := &models.OAuthClient{
		ClientID:     clientID,
		ClientSecret: string(hashedSecret), // Store the hashed secret
		ClientName:   name,
		RedirectURIs: strings.Join(redirectURIs, ","), // Store as comma-separated string
		UserID:       userID,
		Scope:        scope,
	}

	if err := s.db.Create(client).Error; err != nil {
		return nil, "", err
	}

	return client, plainClientSecret, nil
}

// GetClient retrieves an OAuth client by its ID.
func (s *OAuthClientService) GetClient(clientID string) (*models.OAuthClient, error) {
	var client models.OAuthClient
	if err := s.db.Preload("User").Where("client_id = ?", clientID).First(&client).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

// ValidateClientSecret compares a provided secret with the stored hashed secret.
func (s *OAuthClientService) ValidateClientSecret(client *models.OAuthClient, secret string) (bool, error) {
	err := bcrypt.CompareHashAndPassword([]byte(client.ClientSecret), []byte(secret))
	if err == nil {
		return true, nil
	}
	if err == bcrypt.ErrMismatchedHashAndPassword {
		return false, nil
	}
	return false, err
}

// ValidateRedirectURI checks if the provided redirectURI is valid for the client.
// The redirectURI must exactly match one of the registered URIs.
func (s *OAuthClientService) ValidateRedirectURI(client *models.OAuthClient, redirectURI string) (bool, error) {
	if client == nil {
		return false, errors.New("client cannot be nil")
	}
	registeredURIs := strings.Split(client.RedirectURIs, ",")
	for _, uri := range registeredURIs {
		if strings.TrimSpace(uri) == strings.TrimSpace(redirectURI) {
			return true, nil
		}
	}
	return false, nil
}
