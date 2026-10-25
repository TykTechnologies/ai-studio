package grpc

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/studiodb"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Plugins may keep a stored $SECRET/ or $ENV/ reference but not set one:
// creates and updates that add or change one are refused.
func TestPluginAPIRefusesNewReferences(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, studiodb.Migrate(db))
	service := services.NewService(db)
	ctx := context.Background()
	spec := base64.StdEncoding.EncodeToString([]byte(slugTestSpec))

	tools := NewToolsServer(service)
	_, err = tools.CreateTool(ctx, &pb.CreateToolRequest{Name: "T", Description: "d", ToolType: "REST", OasSpec: spec, AuthKey: "$SECRET/PLATFORM_KEY"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "%v", err)

	stored, err := service.CreateTool("Stored", "d", "REST", spec, 0, "apiKey", "$SECRET/TOOL_KEY")
	require.NoError(t, err)
	_, err = tools.UpdateTool(ctx, &pb.UpdateToolRequest{ToolId: uint32(stored.ID), Name: "Stored", Description: "d2", ToolType: "REST", OasSpec: spec, AuthKey: "$SECRET/TOOL_KEY"})
	assert.NoError(t, err, "keeping the stored reference")
	_, err = tools.UpdateTool(ctx, &pb.UpdateToolRequest{ToolId: uint32(stored.ID), Name: "Stored", Description: "d2", ToolType: "REST", OasSpec: spec, AuthKey: "$ENV/TYK_AI_SECRET_KEY"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "%v", err)

	llms := NewLLMManagementServer(service)
	_, err = llms.CreateLLM(ctx, &pb.CreateLLMRequest{Name: "L", Vendor: "openai", ApiKey: "$ENV/OPENAI_KEY", ApiEndpoint: "https://api.openai.com/v1", DefaultModel: "gpt-4o"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "%v", err)
	_, err = llms.CreateLLM(ctx, &pb.CreateLLMRequest{Name: "L2", Vendor: "openai", ApiKey: "sk-plain", ApiEndpoint: "$SECRET/ENDPOINT", DefaultModel: "gpt-4o"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "%v", err)

	datasources := NewDatasourcesServer(service)
	_, err = datasources.CreateDatasource(ctx, &pb.CreateDatasourceRequest{Name: "D", DbSourceType: "pgvector", DbConnString: "postgres://x", DbConnApiKey: "$SECRET/DB_KEY"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "%v", err)
}
