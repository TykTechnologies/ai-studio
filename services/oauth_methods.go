package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// GetValidAccessTokenByToken returns an access token by its token string.
// This implements OAuth support for the ServiceInterface.
func (s *Service) GetValidAccessTokenByToken(token string) (*models.AccessToken, error) {
	accessTokenService := NewAccessTokenService(s.DB)
	return accessTokenService.GetValidAccessTokenByToken(token)
}

// GetOAuthClient returns an OAuth client by its client ID.
// This implements OAuth support for the ServiceInterface.
func (s *Service) GetOAuthClient(clientID string) (*models.OAuthClient, error) {
	oauthClientService := NewOAuthClientService(s.DB)
	return oauthClientService.GetClient(clientID)
}
