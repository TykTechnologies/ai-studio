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

const slugTestSpec = `{"openapi":"3.0.0","info":{"title":"T","version":"1"},"servers":[{"url":"https://api.example.com"}],` +
	`"paths":{"/items":{"get":{"operationId":"listItems","responses":{"200":{"description":"OK"}}}}}}`

// Plugins creating or renaming tools and LLMs through the service API get
// AlreadyExists for a name whose slug another one has.
func TestPluginAPIRefusesDuplicateSlugs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, studiodb.Migrate(db))
	service := services.NewService(db)
	ctx := context.Background()
	spec := base64.StdEncoding.EncodeToString([]byte(slugTestSpec))

	tools := NewToolsServer(service)
	_, err = tools.CreateTool(ctx, &pb.CreateToolRequest{Name: "Page fetcher", Description: "d", ToolType: "REST", OasSpec: spec})
	require.NoError(t, err)
	_, err = tools.CreateTool(ctx, &pb.CreateToolRequest{Name: "page fetcher", Description: "d", ToolType: "REST", OasSpec: spec})
	assert.Equal(t, codes.AlreadyExists, status.Code(err), "%v", err)
	other, err := tools.CreateTool(ctx, &pb.CreateToolRequest{Name: "Other", Description: "d", ToolType: "REST", OasSpec: spec})
	require.NoError(t, err)
	_, err = tools.UpdateTool(ctx, &pb.UpdateToolRequest{ToolId: other.Tool.Id, Name: "PAGE-FETCHER", Description: "d", ToolType: "REST", OasSpec: spec})
	assert.Equal(t, codes.AlreadyExists, status.Code(err), "%v", err)

	llms := NewLLMManagementServer(service)
	_, err = llms.CreateLLM(ctx, &pb.CreateLLMRequest{Name: "GPT 4", Vendor: "openai", ApiKey: "k", ApiEndpoint: "https://api.openai.com/v1", DefaultModel: "gpt-4"})
	require.NoError(t, err)
	_, err = llms.CreateLLM(ctx, &pb.CreateLLMRequest{Name: "gpt-4", Vendor: "openai", ApiKey: "k", ApiEndpoint: "https://api.openai.com/v1", DefaultModel: "gpt-4"})
	assert.Equal(t, codes.AlreadyExists, status.Code(err), "%v", err)
}
