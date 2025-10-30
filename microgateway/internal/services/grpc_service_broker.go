package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// MicrogatewayManagementServer implements the gRPC service for plugin-to-host communication
type MicrogatewayManagementServer struct {
	pb.UnimplementedMicrogatewayManagementServiceServer
	db                *gorm.DB
	gatewayService    GatewayServiceInterface
	budgetService     BudgetServiceInterface
	managementService ManagementServiceInterface
	cryptoService     CryptoServiceInterface
}

// NewMicrogatewayManagementServer creates a new service broker server
func NewMicrogatewayManagementServer(
	db *gorm.DB,
	gatewayService GatewayServiceInterface,
	budgetService BudgetServiceInterface,
	managementService ManagementServiceInterface,
	cryptoService CryptoServiceInterface,
) *MicrogatewayManagementServer {
	return &MicrogatewayManagementServer{
		db:                db,
		gatewayService:    gatewayService,
		budgetService:     budgetService,
		managementService: managementService,
		cryptoService:     cryptoService,
	}
}

// validatePluginScope validates that the calling plugin has the required scope
func (s *MicrogatewayManagementServer) validatePluginScope(ctx context.Context, pluginCtx *pb.PluginContext, requiredScope string) (*database.Plugin, error) {
	if pluginCtx == nil {
		return nil, status.Errorf(codes.InvalidArgument, "plugin context required")
	}

	pluginID := pluginCtx.PluginId
	if pluginID == 0 {
		return nil, status.Errorf(codes.Unauthenticated, "plugin authentication required")
	}

	// Load plugin from database
	var plugin database.Plugin
	if err := s.db.First(&plugin, pluginID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Error().Uint32("plugin_id", pluginID).Msg("Plugin not found during scope validation")
			return nil, status.Errorf(codes.Unauthenticated, "plugin not found")
		}
		log.Error().Err(err).Uint32("plugin_id", pluginID).Msg("Database error during scope validation")
		return nil, status.Errorf(codes.Internal, "authentication error")
	}

	// Check if plugin is active
	if !plugin.IsActive {
		log.Warn().Uint32("plugin_id", pluginID).Str("plugin_name", plugin.Name).Msg("Inactive plugin attempted service access")
		return nil, status.Errorf(codes.PermissionDenied, "plugin is not active")
	}

	// Check if plugin has service access authorized
	if !plugin.HasServiceAccess() {
		log.Warn().
			Uint32("plugin_id", pluginID).
			Str("plugin_name", plugin.Name).
			Msg("Plugin service access not authorized")
		return nil, status.Errorf(codes.PermissionDenied, "service access not authorized for plugin %s", plugin.Name)
	}

	// Check scope authorization
	if !plugin.HasServiceScope(requiredScope) {
		log.Warn().
			Uint32("plugin_id", pluginID).
			Str("plugin_name", plugin.Name).
			Str("required_scope", requiredScope).
			Strs("plugin_scopes", plugin.GetServiceScopes()).
			Msg("Plugin missing required scope")
		return nil, status.Errorf(codes.PermissionDenied, "insufficient scope: %s (plugin: %s)", requiredScope, plugin.Name)
	}

	log.Debug().
		Uint32("plugin_id", pluginID).
		Str("plugin_name", plugin.Name).
		Str("scope", requiredScope).
		Msg("Plugin scope validated successfully")

	return &plugin, nil
}

// LLM Management Operations

func (s *MicrogatewayManagementServer) ListLLMs(ctx context.Context, req *pb.ListLLMsRequest) (*pb.ListLLMsResponse, error) {
	_, err := s.validatePluginScope(ctx, req.Context, "llms.read")
	if err != nil {
		return nil, err
	}

	page := int(req.Page)
	if page <= 0 {
		page = 1
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}

	vendor := req.Vendor
	// Default to active LLMs if not specified
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	llms, total, err := s.managementService.ListLLMs(page, limit, vendor, isActive)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list LLMs")
		return nil, status.Errorf(codes.Internal, "failed to list LLMs: %v", err)
	}

	var pbLLMs []*pb.LLMInfo
	for _, llm := range llms {
		pbLLMs = append(pbLLMs, s.convertLLMToPB(&llm))
	}

	return &pb.ListLLMsResponse{
		Llms:       pbLLMs,
		TotalCount: total,
	}, nil
}

func (s *MicrogatewayManagementServer) GetLLM(ctx context.Context, req *pb.GetLLMRequest) (*pb.GetLLMResponse, error) {
	_, err := s.validatePluginScope(ctx, req.Context, "llms.read")
	if err != nil {
		return nil, err
	}

	llm, err := s.managementService.GetLLM(uint(req.LlmId))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "LLM not found")
		}
		log.Error().Err(err).Uint32("llm_id", req.LlmId).Msg("Failed to get LLM")
		return nil, status.Errorf(codes.Internal, "failed to get LLM: %v", err)
	}

	return &pb.GetLLMResponse{
		Llm: s.convertLLMToPB(llm),
	}, nil
}

// App Management Operations

func (s *MicrogatewayManagementServer) ListApps(ctx context.Context, req *pb.ListAppsRequest) (*pb.ListAppsResponse, error) {
	_, err := s.validatePluginScope(ctx, req.Context, "apps.read")
	if err != nil {
		return nil, err
	}

	page := int(req.Page)
	if page <= 0 {
		page = 1
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 10
	}

	// Default to active apps if not specified
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	apps, total, err := s.managementService.ListApps(page, limit, isActive)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list apps")
		return nil, status.Errorf(codes.Internal, "failed to list apps: %v", err)
	}

	var pbApps []*pb.AppInfo
	for _, app := range apps {
		pbApps = append(pbApps, s.convertAppToPB(&app))
	}

	return &pb.ListAppsResponse{
		Apps:       pbApps,
		TotalCount: total,
	}, nil
}

func (s *MicrogatewayManagementServer) GetApp(ctx context.Context, req *pb.GetAppRequest) (*pb.GetAppResponse, error) {
	_, err := s.validatePluginScope(ctx, req.Context, "apps.read")
	if err != nil {
		return nil, err
	}

	app, err := s.managementService.GetApp(uint(req.AppId))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "app not found")
		}
		log.Error().Err(err).Uint32("app_id", req.AppId).Msg("Failed to get app")
		return nil, status.Errorf(codes.Internal, "failed to get app: %v", err)
	}

	return &pb.GetAppResponse{
		App: s.convertAppToPB(app),
	}, nil
}

// Budget Operations

func (s *MicrogatewayManagementServer) GetBudgetStatus(ctx context.Context, req *pb.GetBudgetStatusRequest) (*pb.GetBudgetStatusResponse, error) {
	_, err := s.validatePluginScope(ctx, req.Context, "budget.read")
	if err != nil {
		return nil, err
	}

	var llmID *uint
	if req.LlmId != nil {
		id := uint(*req.LlmId)
		llmID = &id
	}

	budgetStatus, err := s.budgetService.GetBudgetStatus(uint(req.AppId), llmID)
	if err != nil {
		log.Error().Err(err).Uint32("app_id", req.AppId).Msg("Failed to get budget status")
		return nil, status.Errorf(codes.Internal, "failed to get budget status: %v", err)
	}

	var pbLLMID *uint32
	if budgetStatus.LLMID != nil {
		id := uint32(*budgetStatus.LLMID)
		pbLLMID = &id
	}

	return &pb.GetBudgetStatusResponse{
		AppId:           uint32(budgetStatus.AppID),
		LlmId:           pbLLMID,
		MonthlyBudget:   budgetStatus.MonthlyBudget,
		CurrentUsage:    budgetStatus.CurrentUsage,
		RemainingBudget: budgetStatus.RemainingBudget,
		TokensUsed:      budgetStatus.TokensUsed,
		RequestsCount:   int32(budgetStatus.RequestsCount),
		PeriodStart:     budgetStatus.PeriodStart.Unix(),
		PeriodEnd:       budgetStatus.PeriodEnd.Unix(),
		IsOverBudget:    budgetStatus.IsOverBudget,
		PercentageUsed:  budgetStatus.PercentageUsed,
	}, nil
}

// Model Price Operations

func (s *MicrogatewayManagementServer) ListModelPrices(ctx context.Context, req *pb.ListModelPricesRequest) (*pb.ListModelPricesResponse, error) {
	_, err := s.validatePluginScope(ctx, req.Context, "pricing.read")
	if err != nil {
		return nil, err
	}

	prices, err := s.managementService.ListModelPrices(req.Vendor)
	if err != nil {
		log.Error().Err(err).Str("vendor", req.Vendor).Msg("Failed to list model prices")
		return nil, status.Errorf(codes.Internal, "failed to list model prices: %v", err)
	}

	var pbPrices []*pb.ModelPriceInfo
	for _, price := range prices {
		pbPrices = append(pbPrices, s.convertModelPriceToPB(&price))
	}

	return &pb.ListModelPricesResponse{
		ModelPrices: pbPrices,
	}, nil
}

func (s *MicrogatewayManagementServer) GetModelPrice(ctx context.Context, req *pb.GetModelPriceRequest) (*pb.GetModelPriceResponse, error) {
	_, err := s.validatePluginScope(ctx, req.Context, "pricing.read")
	if err != nil {
		return nil, err
	}

	price, err := s.managementService.GetModelPrice(req.ModelName, req.Vendor)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "model price not found")
		}
		log.Error().Err(err).Str("model", req.ModelName).Str("vendor", req.Vendor).Msg("Failed to get model price")
		return nil, status.Errorf(codes.Internal, "failed to get model price: %v", err)
	}

	return &pb.GetModelPriceResponse{
		ModelPrice: s.convertModelPriceToPB(price),
	}, nil
}

// Credential Operations

func (s *MicrogatewayManagementServer) ValidateCredential(ctx context.Context, req *pb.ValidateCredentialRequest) (*pb.ValidateCredentialResponse, error) {
	_, err := s.validatePluginScope(ctx, req.Context, "credentials.validate")
	if err != nil {
		return nil, err
	}

	credInterface, err := s.gatewayService.GetCredentialBySecret(req.Secret)
	if err != nil {
		// Don't expose details about why validation failed
		return &pb.ValidateCredentialResponse{
			Valid: false,
		}, nil
	}

	cred, ok := credInterface.(*database.Credential)
	if !ok {
		return &pb.ValidateCredentialResponse{
			Valid: false,
		}, nil
	}

	// Check if credential is active and not expired
	if !cred.IsActive {
		return &pb.ValidateCredentialResponse{
			Valid: false,
		}, nil
	}

	if cred.ExpiresAt != nil && cred.ExpiresAt.Before(time.Now()) {
		return &pb.ValidateCredentialResponse{
			Valid: false,
		}, nil
	}

	var expiresAt *int64
	if cred.ExpiresAt != nil {
		exp := cred.ExpiresAt.Unix()
		expiresAt = &exp
	}

	return &pb.ValidateCredentialResponse{
		Valid:        true,
		AppId:        uint32(cred.AppID),
		CredentialId: uint32(cred.ID),
		ExpiresAt:    expiresAt,
	}, nil
}

// Plugin KV Storage Operations

func (s *MicrogatewayManagementServer) WritePluginKV(ctx context.Context, req *pb.WritePluginKVRequest) (*pb.WritePluginKVResponse, error) {
	plugin, err := s.validatePluginScope(ctx, req.Context, "kv.readwrite")
	if err != nil {
		return nil, err
	}

	// Create namespaced key: plugin_<id>_<key>
	namespacedKey := fmt.Sprintf("plugin_%d_%s", plugin.ID, req.Key)

	// Check if key exists
	var existing database.PluginKV
	created := s.db.Where("key = ?", namespacedKey).First(&existing).Error == gorm.ErrRecordNotFound

	// Upsert the key-value pair
	kv := database.PluginKV{
		Key:      namespacedKey,
		Value:    req.Value,
		PluginID: plugin.ID,
	}

	if created {
		if err := s.db.Create(&kv).Error; err != nil {
			log.Error().Err(err).Str("key", req.Key).Msg("Failed to write plugin KV")
			return nil, status.Errorf(codes.Internal, "failed to write KV data")
		}
	} else {
		if err := s.db.Model(&existing).Updates(map[string]interface{}{
			"value":      req.Value,
			"updated_at": time.Now(),
		}).Error; err != nil {
			log.Error().Err(err).Str("key", req.Key).Msg("Failed to update plugin KV")
			return nil, status.Errorf(codes.Internal, "failed to update KV data")
		}
	}

	return &pb.WritePluginKVResponse{
		Created: created,
	}, nil
}

func (s *MicrogatewayManagementServer) ReadPluginKV(ctx context.Context, req *pb.ReadPluginKVRequest) (*pb.ReadPluginKVResponse, error) {
	plugin, err := s.validatePluginScope(ctx, req.Context, "kv.readwrite")
	if err != nil {
		return nil, err
	}

	// Create namespaced key: plugin_<id>_<key>
	namespacedKey := fmt.Sprintf("plugin_%d_%s", plugin.ID, req.Key)

	var kv database.PluginKV
	if err := s.db.Where("key = ?", namespacedKey).First(&kv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "key not found")
		}
		log.Error().Err(err).Str("key", req.Key).Msg("Failed to read plugin KV")
		return nil, status.Errorf(codes.Internal, "failed to read KV data")
	}

	return &pb.ReadPluginKVResponse{
		Value: kv.Value,
	}, nil
}

func (s *MicrogatewayManagementServer) DeletePluginKV(ctx context.Context, req *pb.DeletePluginKVRequest) (*pb.DeletePluginKVResponse, error) {
	plugin, err := s.validatePluginScope(ctx, req.Context, "kv.readwrite")
	if err != nil {
		return nil, err
	}

	// Create namespaced key: plugin_<id>_<key>
	namespacedKey := fmt.Sprintf("plugin_%d_%s", plugin.ID, req.Key)

	result := s.db.Where("key = ?", namespacedKey).Delete(&database.PluginKV{})
	if result.Error != nil {
		log.Error().Err(result.Error).Str("key", req.Key).Msg("Failed to delete plugin KV")
		return nil, status.Errorf(codes.Internal, "failed to delete KV data")
	}

	return &pb.DeletePluginKVResponse{
		Deleted: result.RowsAffected > 0,
	}, nil
}

// Helper conversion functions

func (s *MicrogatewayManagementServer) convertLLMToPB(llm *database.LLM) *pb.LLMInfo {
	var allowedModels []string
	if llm.AllowedModels != nil {
		json.Unmarshal(llm.AllowedModels, &allowedModels)
	}

	var monthlyBudget *float64
	if llm.MonthlyBudget > 0 {
		monthlyBudget = &llm.MonthlyBudget
	}

	return &pb.LLMInfo{
		Id:             uint32(llm.ID),
		Name:           llm.Name,
		Vendor:         llm.Vendor,
		Slug:           llm.Slug,
		Endpoint:       llm.Endpoint,
		HasApiKey:      llm.APIKeyEncrypted != "",
		DefaultModel:   llm.DefaultModel,
		AllowedModels:  allowedModels,
		IsActive:       llm.IsActive,
		MaxTokens:      int32(llm.MaxTokens),
		TimeoutSeconds: int32(llm.TimeoutSeconds),
		RetryCount:     int32(llm.RetryCount),
		MonthlyBudget:  monthlyBudget,
		RateLimitRpm:   int32(llm.RateLimitRPM),
		CreatedAt:      timestamppb.New(llm.CreatedAt),
		UpdatedAt:      timestamppb.New(llm.UpdatedAt),
	}
}

func (s *MicrogatewayManagementServer) convertAppToPB(app *database.App) *pb.AppInfo {
	var allowedIPs []string
	if app.AllowedIPs != nil {
		json.Unmarshal(app.AllowedIPs, &allowedIPs)
	}

	// Convert metadata to JSON string
	var metadataJSON string
	if app.Metadata != nil && len(app.Metadata) > 0 {
		metadataJSON = string(app.Metadata)
	}

	return &pb.AppInfo{
		Id:             uint32(app.ID),
		Name:           app.Name,
		Description:    app.Description,
		OwnerEmail:     app.OwnerEmail,
		IsActive:       app.IsActive,
		MonthlyBudget:  app.MonthlyBudget,
		BudgetResetDay: int32(app.BudgetResetDay),
		RateLimitRpm:   int32(app.RateLimitRPM),
		AllowedIps:     allowedIPs,
		Metadata:       metadataJSON,
		CreatedAt:      timestamppb.New(app.CreatedAt),
		UpdatedAt:      timestamppb.New(app.UpdatedAt),
	}
}

func (s *MicrogatewayManagementServer) convertModelPriceToPB(price *database.ModelPrice) *pb.ModelPriceInfo {
	return &pb.ModelPriceInfo{
		Id:           uint32(price.ID),
		Vendor:       price.Vendor,
		ModelName:    price.ModelName,
		Cpt:          price.CPT,
		Cpit:         price.CPIT,
		CacheWritePt: price.CacheWritePT,
		CacheReadPt:  price.CacheReadPT,
		Currency:     price.Currency,
		CreatedAt:    timestamppb.New(price.CreatedAt),
		UpdatedAt:    timestamppb.New(price.UpdatedAt),
	}
}
