package entitlements

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExceededErrorConvertsToConflictEvenWhenWrapped(t *testing.T) {
	err := fmt.Errorf("create app: %w", NewExceededError("no room left"))

	var he helpers.ErrorResponse
	require.True(t, errors.As(err, &he))
	assert.Equal(t, http.StatusConflict, he.StatusCode)
	assert.Equal(t, CodeQuotaExceeded, he.Code)
	assert.Equal(t, "no room left", he.Message)

	ee, ok := AsExceeded(err)
	require.True(t, ok)
	assert.Equal(t, "no room left", ee.Message)

	_, ok = AsExceeded(errors.New("other"))
	assert.False(t, ok)
	assert.False(t, errors.As(errors.New("other"), &he))
}

func TestExceededErrorIsResourceExhaustedOverGRPC(t *testing.T) {
	err := fmt.Errorf("create app: %w", NewExceededError("no room left"))

	// A handler returning the error as is.
	st := status.Convert(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())

	// The explicit helper.
	gerr, ok := GRPCError(err)
	require.True(t, ok)
	st = status.Convert(gerr)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
	assert.Equal(t, "no room left", st.Message())

	_, ok = GRPCError(errors.New("other"))
	assert.False(t, ok)
}

func TestCommunityStubAllowsEverything(t *testing.T) {
	if IsEnterpriseAvailable() {
		t.Skip("enterprise implementation registered")
	}
	s := NewService(Deps{})

	ran := 0
	fn := func(*gorm.DB) error { ran++; return nil }
	require.NoError(t, s.GuardAppCreate(nil, 1, true, fn))
	require.NoError(t, s.GuardUserChange(nil, fn))
	require.NoError(t, s.GuardLLMCreate(nil, fn))
	assert.Equal(t, 3, ran)

	// The guard returns fn's own error unchanged.
	boom := errors.New("boom")
	assert.Equal(t, boom, s.GuardAppCreate(nil, 1, false, func(*gorm.DB) error { return boom }))
	assert.Equal(t, boom, s.GuardLLMCreate(nil, func(*gorm.DB) error { return boom }))

	licensed, err := s.AdmitNamespace(nil, "team-a")
	require.NoError(t, err)
	assert.True(t, licensed)
	assert.True(t, s.IsNamespaceLicensed(nil, "team-a"))
	assert.Nil(t, s.LicensedNamespaceScope(nil))
	assert.True(t, s.CanGrant(nil, Seat{Portal: true, Admin: true, GroupID: 3}))

	_, err = s.Report(nil)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = s.Notices(nil, nil)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	assert.ErrorIs(t, s.DismissNotice(nil, nil, "k"), ErrEnterpriseFeature)
	assert.ErrorIs(t, s.ReleaseNamespace(nil, "team-a"), ErrEnterpriseFeature)
	_, err = s.GetTeamAppQuota(nil, 1)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	assert.ErrorIs(t, s.SetTeamAppQuota(nil, 1, TeamAppQuotaInput{Unlimited: true}), ErrEnterpriseFeature)
	assert.ErrorIs(t, s.DeleteTeamAppQuota(nil, 1), ErrEnterpriseFeature)
	_, _, _, err = s.UserAppQuota(nil, 1)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
}
