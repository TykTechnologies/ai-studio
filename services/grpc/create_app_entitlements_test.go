package grpc

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type refusingEntitlements struct {
	entitlements.Service
	selfService []bool
	refuse      string
}

func (r *refusingEntitlements) GuardAppCreate(tx *gorm.DB, _ uint, selfService bool, fn func(*gorm.DB) error) error {
	r.selfService = append(r.selfService, selfService)
	if r.refuse != "" {
		return entitlements.NewExceededError(r.refuse)
	}
	return fn(tx)
}

// Plugin SDK creates are self-service, and a refusal reaches the plugin as
// ResourceExhausted with the entitlements message rather than Internal.
func TestCreateAppGuardedAsSelfServiceOverGRPC(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	service := services.NewService(db)
	ent := &refusingEntitlements{Service: entitlements.NewCommunityService()}
	service.Entitlements = ent
	server := NewAIStudioManagementServer(service)
	owner := &models.User{Email: "grpc-quota@example.com", Name: "Owner"}
	require.NoError(t, owner.Create(db))

	_, err = server.CreateApp(context.Background(), &pb.CreateAppRequest{Name: "first", UserId: uint32(owner.ID)})
	require.NoError(t, err)

	ent.refuse = "Your organisation has reached its licensed limit of 1 app."
	_, err = server.CreateApp(context.Background(), &pb.CreateAppRequest{Name: "second", UserId: uint32(owner.ID)})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
	assert.Equal(t, ent.refuse, st.Message())

	assert.Equal(t, []bool{true, true}, ent.selfService)
}
