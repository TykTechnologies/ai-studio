//go:build !enterprise
// +build !enterprise

package sso

import (
	"github.com/TykTechnologies/midsommar/v2/models"
)

// communityService is a stub implementation for Community Edition
// All methods return errors indicating SSO is an enterprise feature
type communityService struct{}

func newCommunityService() Service {
	return &communityService{}
}

func (s *communityService) InitInternalTIB() error {
	// CE: No-op, SSO not available
	return nil
}

func (s *communityService) GetTapProfile(id string) (TAPProvider, *TAPProfile, error) {
	return nil, nil, ErrSSONotAvailable
}

func (s *communityService) GenerateNonce(request NonceTokenRequest) (*string, error) {
	return nil, ErrSSONotAvailable
}

func (s *communityService) ValidateNonceRequest(request *NonceTokenRequest) error {
	return ErrSSONotAvailable
}

func (s *communityService) ResolveNonce(token string, consume bool) (*NonceTokenRequest, error) {
	return nil, ErrSSONotAvailable
}

func (s *communityService) HandleSSO(emailAddress, displayName, groupID string, groupsIDs []string, ssoOnlyForRegisteredUsers bool) (*models.User, error) {
	return nil, ErrSSONotAvailable
}
