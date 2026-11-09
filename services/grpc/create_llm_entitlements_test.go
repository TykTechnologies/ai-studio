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

type refusingLLMEntitlements struct {
	entitlements.Service
	refuse string
}

func (r *refusingLLMEntitlements) GuardLLMCreate(tx *gorm.DB, fn func(*gorm.DB) error) error {
	if r.refuse != "" {
		return entitlements.NewExceededError(r.refuse)
	}
	return tx.Transaction(fn)
}

// A plugin SDK LLM create over the licence limit gets ResourceExhausted with
// the entitlements message rather than Internal.
func TestCreateLLMGuardedOverGRPC(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	service := services.NewService(db)
	ent := &refusingLLMEntitlements{Service: entitlements.NewCommunityService()}
	service.Entitlements = ent
	server := NewAIStudioManagementServer(service)

	req := func(name string) *pb.CreateLLMRequest {
		return &pb.CreateLLMRequest{Name: name, Vendor: string(models.OPENAI), DefaultModel: "gpt-4o", ApiEndpoint: "https://api.openai.com/v1"}
	}
	_, err = server.CreateLLM(context.Background(), req("first"))
	require.NoError(t, err)

	ent.refuse = "Your organisation has reached its licensed limit of 1 LLM."
	_, err = server.CreateLLM(context.Background(), req("second"))
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
	assert.Equal(t, ent.refuse, st.Message())

	var n int64
	require.NoError(t, db.Model(&models.LLM{}).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}
