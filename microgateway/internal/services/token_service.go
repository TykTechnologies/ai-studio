// internal/services/token_service.go
package services

import (
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
)

// TokenService implements TokenServiceInterface
type TokenService struct {
	authProvider auth.AuthProvider
}

// NewTokenService creates a new token service
func NewTokenService(authProvider auth.AuthProvider) TokenServiceInterface {
	return &TokenService{
		authProvider: authProvider,
	}
}

// GenerateToken generates a new API token
func (s *TokenService) GenerateToken(req *GenerateTokenRequest) (*TokenResponse, error) {
	token, err := s.authProvider.GenerateToken(req.AppID, req.Name, req.Scopes, req.ExpiresIn)
	if err != nil {
		return nil, err
	}

	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		exp := time.Now().Add(req.ExpiresIn)
		expiresAt = &exp
	}

	return &TokenResponse{
		Token:     token,
		Name:      req.Name,
		AppID:     req.AppID,
		Scopes:    req.Scopes,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}, nil
}

// ListTokens lists tokens for an app
func (s *TokenService) ListTokens(appID uint) ([]TokenInfo, error) {
	if tokenAuth, ok := s.authProvider.(*auth.TokenAuthProvider); ok {
		authTokens, err := tokenAuth.ListTokensForApp(appID)
		if err != nil {
			return nil, err
		}
		// Convert auth.TokenInfo to services.TokenInfo
		tokens := make([]TokenInfo, len(authTokens))
		for i, at := range authTokens {
			tokens[i] = TokenInfo{
				ID:        at.ID,
				Name:      at.Name,
				AppID:     at.AppID,
				Scopes:    at.Scopes,
				IsActive:  at.IsActive,
				ExpiresAt: at.ExpiresAt,
				CreatedAt: at.CreatedAt,
				LastUsed:  at.LastUsed,
			}
		}
		return tokens, nil
	}
	return []TokenInfo{}, nil
}

// RevokeToken revokes an API token
func (s *TokenService) RevokeToken(token string) error {
	return s.authProvider.RevokeToken(token)
}

// GetTokenInfo gets information about a token
func (s *TokenService) GetTokenInfo(token string) (*TokenInfo, error) {
	authTokenInfo, err := s.authProvider.GetTokenInfo(token)
	if err != nil {
		return nil, err
	}

	// Convert auth.TokenInfo to services.TokenInfo
	return &TokenInfo{
		ID:        authTokenInfo.ID,
		Name:      authTokenInfo.Name,
		AppID:     authTokenInfo.AppID,
		Scopes:    authTokenInfo.Scopes,
		IsActive:  authTokenInfo.IsActive,
		ExpiresAt: authTokenInfo.ExpiresAt,
		CreatedAt: authTokenInfo.CreatedAt,
		LastUsed:  authTokenInfo.LastUsed,
	}, nil
}