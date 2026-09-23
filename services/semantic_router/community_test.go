package semantic_router

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/stretchr/testify/assert"
)

func TestCommunityService_IsEnterpriseOnly(t *testing.T) {
	s := newCommunityService()
	assert.ErrorIs(t, s.CreateRouter(&models.SemanticRouter{}), ErrEnterpriseFeature)
	_, err := s.GetRouter(1)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	assert.ErrorIs(t, s.UpdateRouter(&models.SemanticRouter{}), ErrEnterpriseFeature)
	assert.ErrorIs(t, s.DeleteRouter(1), ErrEnterpriseFeature)
	_, _, _, err = s.ListRouters(10, 1, false)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	assert.ErrorIs(t, s.ToggleRouterActive(1, true), ErrEnterpriseFeature)
	assert.ErrorIs(t, s.ValidateRouter(&models.SemanticRouter{}), ErrEnterpriseFeature)
	_, err = s.Test(context.Background(), &models.SemanticRouter{}, sr.Request{})
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
}
