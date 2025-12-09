// +build !no_microgateway_sdk

package plugin_sdk

import (
	"context"
	"time"

	mgwsdk "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// initializeMicrogatewaySDK initializes the Microgateway SDK for service API access
func initializeMicrogatewaySDK(server *grpc.Server, broker *goplugin.GRPCBroker, pluginID uint32) error {
	return mgwsdk.InitializeServiceAPI(server, broker, pluginID)
}

// gatewayServicesImpl provides Gateway-specific services using the microgateway SDK
type gatewayServicesImpl struct{}

func (g *gatewayServicesImpl) GetApp(ctx context.Context, appID uint32) (interface{}, error) {
	return mgwsdk.GetApp(ctx, appID)
}

func (g *gatewayServicesImpl) ListApps(ctx context.Context, page, limit int32, isActive *bool) (interface{}, error) {
	return mgwsdk.ListApps(ctx, page, limit, isActive)
}

func (g *gatewayServicesImpl) GetLLM(ctx context.Context, llmID uint32) (interface{}, error) {
	return mgwsdk.GetLLM(ctx, llmID)
}

func (g *gatewayServicesImpl) ListLLMs(ctx context.Context, page, limit int32, vendor string, isActive *bool) (interface{}, error) {
	return mgwsdk.ListLLMs(ctx, page, limit, vendor, isActive)
}

func (g *gatewayServicesImpl) GetBudgetStatus(ctx context.Context, appID uint32, llmID *uint32) (interface{}, error) {
	return mgwsdk.GetBudgetStatus(ctx, appID, llmID)
}

func (g *gatewayServicesImpl) GetModelPrice(ctx context.Context, modelName, vendor string) (interface{}, error) {
	return mgwsdk.GetModelPrice(ctx, modelName, vendor)
}

func (g *gatewayServicesImpl) ListModelPrices(ctx context.Context, vendor string) (interface{}, error) {
	return mgwsdk.ListModelPrices(ctx, vendor)
}

func (g *gatewayServicesImpl) ValidateCredential(ctx context.Context, secret string) (interface{}, error) {
	return mgwsdk.ValidateCredential(ctx, secret)
}

func (g *gatewayServicesImpl) SendToControl(ctx context.Context, payload []byte, correlationID string, metadata map[string]string) (int64, error) {
	return mgwsdk.SendToControl(ctx, payload, correlationID, metadata)
}

func (g *gatewayServicesImpl) SendToControlJSON(ctx context.Context, value interface{}, correlationID string, metadata map[string]string) (int64, error) {
	return mgwsdk.SendToControlJSON(ctx, value, correlationID, metadata)
}

// setBrokerIDForMicrogatewaySDK sets the broker ID for the Microgateway SDK
func setBrokerIDForMicrogatewaySDK(brokerID uint32) {
	mgwsdk.SetServiceBrokerID(brokerID)
}

// setPluginIDForMicrogatewaySDK sets the plugin ID for the Microgateway SDK
func setPluginIDForMicrogatewaySDK(pluginID uint32) {
	mgwsdk.SetPluginID(pluginID)
}

// ===== KV Service Wrappers for Gateway =====

// readKVGateway wraps the Microgateway SDK's KV read
func readKVGateway(ctx context.Context, key string) ([]byte, error) {
	return mgwsdk.ReadPluginKV(ctx, key)
}

// writeKVGateway wraps the Microgateway SDK's KV write
func writeKVGateway(ctx context.Context, key string, value []byte, expireAt *time.Time) (bool, error) {
	return mgwsdk.WritePluginKV(ctx, key, value, expireAt)
}

// deleteKVGateway wraps the Microgateway SDK's KV delete
func deleteKVGateway(ctx context.Context, key string) (bool, error) {
	return mgwsdk.DeletePluginKV(ctx, key)
}

// getLicenseInfoGateway retrieves license info from the Microgateway
func getLicenseInfoGateway(ctx context.Context) (*LicenseInfo, error) {
	info, err := mgwsdk.GetLicenseInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &LicenseInfo{
		Valid:         info.Valid,
		DaysRemaining: info.DaysLeft,
		Type:          info.Type,
		Entitlements:  info.Entitlements,
		Organization:  info.Organization,
		// Note: Microgateway SDK doesn't include ExpiresAt, so it stays as zero value
	}, nil
}
