// +build no_microgateway_sdk

package plugin_sdk

import (
	"context"
	"fmt"

	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// initializeMicrogatewaySDK stub - returns nil when microgateway SDK not available
func initializeMicrogatewaySDK(server *grpc.Server, broker *goplugin.GRPCBroker, pluginID uint32) error {
	// No-op in Studio-only builds
	return nil
}

// setBrokerIDForMicrogatewaySDK stub - no-op when microgateway SDK not available
func setBrokerIDForMicrogatewaySDK(brokerID uint32) {
	// No-op in Studio-only builds
}

// gatewayServicesImpl stub - returns errors when called in Studio context
type gatewayServicesImpl struct{}

func (g *gatewayServicesImpl) GetApp(ctx context.Context, appID uint32) (interface{}, error) {
	return nil, fmt.Errorf("gateway services not available in this context")
}

func (g *gatewayServicesImpl) ListApps(ctx context.Context, page, limit int32, isActive *bool) (interface{}, error) {
	return nil, fmt.Errorf("gateway services not available in this context")
}

func (g *gatewayServicesImpl) GetLLM(ctx context.Context, llmID uint32) (interface{}, error) {
	return nil, fmt.Errorf("gateway services not available in this context")
}

func (g *gatewayServicesImpl) ListLLMs(ctx context.Context, page, limit int32, vendor string, isActive *bool) (interface{}, error) {
	return nil, fmt.Errorf("gateway services not available in this context")
}

func (g *gatewayServicesImpl) GetBudgetStatus(ctx context.Context, appID uint32, llmID *uint32) (interface{}, error) {
	return nil, fmt.Errorf("gateway services not available in this context")
}

func (g *gatewayServicesImpl) GetModelPrice(ctx context.Context, modelName, vendor string) (interface{}, error) {
	return nil, fmt.Errorf("gateway services not available in this context")
}

func (g *gatewayServicesImpl) ListModelPrices(ctx context.Context, vendor string) (interface{}, error) {
	return nil, fmt.Errorf("gateway services not available in this context")
}

func (g *gatewayServicesImpl) ValidateCredential(ctx context.Context, secret string) (interface{}, error) {
	return nil, fmt.Errorf("gateway services not available in this context")
}

// setPluginIDForMicrogatewaySDK stub - no-op when microgateway SDK not available
func setPluginIDForMicrogatewaySDK(pluginID uint32) {
	// No-op in Studio-only builds
}

// KV stubs for Studio-only builds
func readKVGateway(ctx context.Context, key string) ([]byte, error) {
	return nil, fmt.Errorf("gateway KV not available in this build")
}

func writeKVGateway(ctx context.Context, key string, value []byte) (bool, error) {
	return false, fmt.Errorf("gateway KV not available in this build")
}

func deleteKVGateway(ctx context.Context, key string) (bool, error) {
	return false, fmt.Errorf("gateway KV not available in this build")
}
