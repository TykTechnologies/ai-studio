package plugintest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	gwmgmtpb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	eventpb "github.com/TykTechnologies/midsommar/v2/proto/plugin_events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestManagementServer implements the AI Studio management service for testing.
// It tracks all calls made by the plugin for assertion purposes.
type TestManagementServer struct {
	mgmtpb.UnimplementedAIStudioManagementServiceServer

	// License configuration
	license        *LicenseInfo
	licenseChecked bool

	// KV storage
	kvStore  map[string][]byte
	kvWrites []KVWrite

	// Call tracking
	calls []ServiceCall

	mu sync.RWMutex
}

// ServiceCall represents a tracked service call.
type ServiceCall struct {
	Method    string
	Request   interface{}
	Timestamp time.Time
}

// NewTestManagementServer creates a new test management server.
func NewTestManagementServer() *TestManagementServer {
	return &TestManagementServer{
		kvStore:  make(map[string][]byte),
		kvWrites: []KVWrite{},
		calls:    []ServiceCall{},
		license: &LicenseInfo{
			Valid:         true,
			DaysRemaining: 365,
			Type:          "enterprise",
			Entitlements:  []string{"advanced-llm-cache"},
			Organization:  "Test Organization",
		},
	}
}

// GetLicenseInfo implements the GetLicenseInfo RPC.
func (s *TestManagementServer) GetLicenseInfo(ctx context.Context, req *mgmtpb.GetLicenseInfoRequest) (*mgmtpb.GetLicenseInfoResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.licenseChecked = true
	s.calls = append(s.calls, ServiceCall{
		Method:    "GetLicenseInfo",
		Request:   req,
		Timestamp: time.Now(),
	})

	if s.license == nil {
		return &mgmtpb.GetLicenseInfoResponse{
			LicenseValid:  false,
			DaysRemaining: 0,
			LicenseType:   "community",
		}, nil
	}

	resp := &mgmtpb.GetLicenseInfoResponse{
		LicenseValid:  s.license.Valid,
		DaysRemaining: int32(s.license.DaysRemaining),
		LicenseType:   s.license.Type,
		Entitlements:  s.license.Entitlements,
		Organization:  s.license.Organization,
	}

	if s.license.DaysRemaining > 0 {
		resp.ExpiresAt = timestamppb.New(time.Now().AddDate(0, 0, s.license.DaysRemaining))
	}

	return resp, nil
}

// ReadPluginKV implements KV read for plugins.
func (s *TestManagementServer) ReadPluginKV(ctx context.Context, req *mgmtpb.ReadPluginKVRequest) (*mgmtpb.ReadPluginKVResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "ReadPluginKV",
		Request:   req,
		Timestamp: time.Now(),
	})

	value, exists := s.kvStore[req.Key]
	msg := ""
	if !exists {
		msg = "key not found"
	}
	return &mgmtpb.ReadPluginKVResponse{
		Value:   value,
		Message: msg,
	}, nil
}

// WritePluginKV implements KV write for plugins.
func (s *TestManagementServer) WritePluginKV(ctx context.Context, req *mgmtpb.WritePluginKVRequest) (*mgmtpb.WritePluginKVResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "WritePluginKV",
		Request:   req,
		Timestamp: time.Now(),
	})

	_, existed := s.kvStore[req.Key]
	s.kvStore[req.Key] = req.Value

	var expireAt *time.Time
	if req.ExpireAt != nil {
		t := req.ExpireAt.AsTime()
		expireAt = &t
	}

	s.kvWrites = append(s.kvWrites, KVWrite{
		Key:       req.Key,
		Value:     req.Value,
		ExpireAt:  expireAt,
		Timestamp: time.Now(),
	})

	return &mgmtpb.WritePluginKVResponse{
		Created: !existed,
	}, nil
}

// DeletePluginKV implements KV delete for plugins.
func (s *TestManagementServer) DeletePluginKV(ctx context.Context, req *mgmtpb.DeletePluginKVRequest) (*mgmtpb.DeletePluginKVResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "DeletePluginKV",
		Request:   req,
		Timestamp: time.Now(),
	})

	_, existed := s.kvStore[req.Key]
	delete(s.kvStore, req.Key)

	return &mgmtpb.DeletePluginKVResponse{
		Deleted: existed,
	}, nil
}

// UpdatePluginConfig implements the UpdatePluginConfig RPC.
// Returns success: true to allow config updates in e2e tests.
func (s *TestManagementServer) UpdatePluginConfig(ctx context.Context, req *mgmtpb.UpdatePluginConfigRequest) (*mgmtpb.UpdatePluginConfigResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "UpdatePluginConfig",
		Request:   req,
		Timestamp: time.Now(),
	})

	return &mgmtpb.UpdatePluginConfigResponse{
		Success: true,
		Message: "Configuration saved (test)",
	}, nil
}

// GetCalls returns all tracked service calls.
func (s *TestManagementServer) GetCalls() []ServiceCall {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ServiceCall, len(s.calls))
	copy(result, s.calls)
	return result
}

// GetCallsByMethod returns calls filtered by method name.
func (s *TestManagementServer) GetCallsByMethod(method string) []ServiceCall {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []ServiceCall
	for _, call := range s.calls {
		if call.Method == method {
			result = append(result, call)
		}
	}
	return result
}

// Reset clears all tracking data.
func (s *TestManagementServer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.licenseChecked = false
	s.kvStore = make(map[string][]byte)
	s.kvWrites = []KVWrite{}
	s.calls = []ServiceCall{}
}

// ============================================================================
// Test Event Service
// ============================================================================

// TestEventService implements the plugin event service for testing.
type TestEventService struct {
	eventpb.UnimplementedPluginEventServiceServer

	publishedEvents []Event
	subscriptions   map[string][]EventCallback
	injectedEvents  chan *eventpb.EventMessage

	mu sync.RWMutex
}

// EventCallback is a function called when an event is received.
type EventCallback func(event *eventpb.EventMessage)

// NewTestEventService creates a new test event service.
func NewTestEventService() *TestEventService {
	return &TestEventService{
		publishedEvents: []Event{},
		subscriptions:   make(map[string][]EventCallback),
		injectedEvents:  make(chan *eventpb.EventMessage, 100),
	}
}

// Publish implements the Publish RPC.
func (s *TestEventService) Publish(ctx context.Context, req *eventpb.PublishRequest) (*eventpb.PublishResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.publishedEvents = append(s.publishedEvents, Event{
		Topic:     req.Topic,
		Payload:   req.Payload,
		Direction: req.Direction,
		Timestamp: time.Now(),
	})

	return &eventpb.PublishResponse{
		Success: true,
		EventId: generateEventID(),
	}, nil
}

// Subscribe implements the Subscribe streaming RPC.
func (s *TestEventService) Subscribe(req *eventpb.SubscribeRequest, stream eventpb.PluginEventService_SubscribeServer) error {
	// For testing, just keep the stream open and send injected events
	for {
		select {
		case event := <-s.injectedEvents:
			// Check if this event matches the subscription
			topicMatches := false
			if req.SubscribeAll {
				topicMatches = true
			} else if req.Topic == event.Topic || req.Topic == "*" || req.Topic == "" {
				topicMatches = true
			}

			if topicMatches {
				if err := stream.Send(event); err != nil {
					return err
				}
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// GetPublishedEvents returns all published events.
func (s *TestEventService) GetPublishedEvents() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Event, len(s.publishedEvents))
	copy(result, s.publishedEvents)
	return result
}

// GetPublishedEventsByTopic returns published events filtered by topic.
func (s *TestEventService) GetPublishedEventsByTopic(topic string) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Event
	for _, event := range s.publishedEvents {
		if event.Topic == topic {
			result = append(result, event)
		}
	}
	return result
}

// InjectEvent simulates receiving an event from the event bus.
func (s *TestEventService) InjectEvent(topic string, payload []byte) {
	s.injectedEvents <- &eventpb.EventMessage{
		Id:      generateEventID(),
		Topic:   topic,
		Payload: payload,
		Origin:  "test-harness",
	}
}

// Reset clears all published events.
func (s *TestEventService) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishedEvents = []Event{}
}

// ============================================================================
// Test Gateway Management Server
// ============================================================================

// TestGatewayManagementServer implements the Microgateway management service for testing.
// It tracks all calls made by the plugin for assertion purposes.
type TestGatewayManagementServer struct {
	gwmgmtpb.UnimplementedMicrogatewayManagementServiceServer

	// License configuration (shared with TestManagementServer)
	license        *LicenseInfo
	licenseChecked bool

	// KV storage
	kvStore  map[string][]byte
	kvWrites []KVWrite

	// Test data for gateway-specific operations
	apps         map[uint32]*gwmgmtpb.AppInfo
	llms         map[uint32]*gwmgmtpb.LLMInfo
	budgetStatus map[uint32]*gwmgmtpb.GetBudgetStatusResponse
	modelPrices  []*gwmgmtpb.ModelPriceInfo

	// Control payload queue
	controlPayloads []ControlPayload

	// Call tracking
	calls []ServiceCall

	mu sync.RWMutex
}

// ControlPayload represents a queued control payload for tracking.
type ControlPayload struct {
	CorrelationID string
	Payload       []byte
	Metadata      map[string]string
	Timestamp     time.Time
}

// NewTestGatewayManagementServer creates a new test gateway management server.
func NewTestGatewayManagementServer() *TestGatewayManagementServer {
	return &TestGatewayManagementServer{
		kvStore:         make(map[string][]byte),
		kvWrites:        []KVWrite{},
		calls:           []ServiceCall{},
		apps:            make(map[uint32]*gwmgmtpb.AppInfo),
		llms:            make(map[uint32]*gwmgmtpb.LLMInfo),
		budgetStatus:    make(map[uint32]*gwmgmtpb.GetBudgetStatusResponse),
		modelPrices:     []*gwmgmtpb.ModelPriceInfo{},
		controlPayloads: []ControlPayload{},
		license: &LicenseInfo{
			Valid:         true,
			DaysRemaining: 365,
			Type:          "enterprise",
			Entitlements:  []string{"advanced-llm-cache"},
			Organization:  "Test Organization",
		},
	}
}

// GetLicenseInfo implements the GetLicenseInfo RPC.
func (s *TestGatewayManagementServer) GetLicenseInfo(ctx context.Context, req *gwmgmtpb.GetLicenseInfoRequest) (*gwmgmtpb.GetLicenseInfoResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.licenseChecked = true
	s.calls = append(s.calls, ServiceCall{
		Method:    "GetLicenseInfo",
		Request:   req,
		Timestamp: time.Now(),
	})

	if s.license == nil {
		return &gwmgmtpb.GetLicenseInfoResponse{
			LicenseValid:  false,
			DaysRemaining: 0,
			LicenseType:   "community",
		}, nil
	}

	resp := &gwmgmtpb.GetLicenseInfoResponse{
		LicenseValid:  s.license.Valid,
		DaysRemaining: int32(s.license.DaysRemaining),
		LicenseType:   s.license.Type,
		Entitlements:  s.license.Entitlements,
		Organization:  s.license.Organization,
	}

	if s.license.DaysRemaining > 0 {
		resp.ExpiresAt = timestamppb.New(time.Now().AddDate(0, 0, s.license.DaysRemaining))
	}

	return resp, nil
}

// ReadPluginKV implements KV read for plugins.
func (s *TestGatewayManagementServer) ReadPluginKV(ctx context.Context, req *gwmgmtpb.ReadPluginKVRequest) (*gwmgmtpb.ReadPluginKVResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "ReadPluginKV",
		Request:   req,
		Timestamp: time.Now(),
	})

	value := s.kvStore[req.Key]
	return &gwmgmtpb.ReadPluginKVResponse{
		Value: value,
	}, nil
}

// WritePluginKV implements KV write for plugins.
func (s *TestGatewayManagementServer) WritePluginKV(ctx context.Context, req *gwmgmtpb.WritePluginKVRequest) (*gwmgmtpb.WritePluginKVResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "WritePluginKV",
		Request:   req,
		Timestamp: time.Now(),
	})

	_, existed := s.kvStore[req.Key]
	s.kvStore[req.Key] = req.Value

	var expireAt *time.Time
	if req.ExpireAt != nil {
		t := req.ExpireAt.AsTime()
		expireAt = &t
	}

	s.kvWrites = append(s.kvWrites, KVWrite{
		Key:       req.Key,
		Value:     req.Value,
		ExpireAt:  expireAt,
		Timestamp: time.Now(),
	})

	return &gwmgmtpb.WritePluginKVResponse{
		Created: !existed,
	}, nil
}

// DeletePluginKV implements KV delete for plugins.
func (s *TestGatewayManagementServer) DeletePluginKV(ctx context.Context, req *gwmgmtpb.DeletePluginKVRequest) (*gwmgmtpb.DeletePluginKVResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "DeletePluginKV",
		Request:   req,
		Timestamp: time.Now(),
	})

	_, existed := s.kvStore[req.Key]
	delete(s.kvStore, req.Key)

	return &gwmgmtpb.DeletePluginKVResponse{
		Deleted: existed,
	}, nil
}

// ListLLMs implements the ListLLMs RPC.
func (s *TestGatewayManagementServer) ListLLMs(ctx context.Context, req *gwmgmtpb.ListLLMsRequest) (*gwmgmtpb.ListLLMsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "ListLLMs",
		Request:   req,
		Timestamp: time.Now(),
	})

	var llms []*gwmgmtpb.LLMInfo
	for _, llm := range s.llms {
		llms = append(llms, llm)
	}

	return &gwmgmtpb.ListLLMsResponse{
		Llms:       llms,
		TotalCount: int64(len(llms)),
	}, nil
}

// GetLLM implements the GetLLM RPC.
func (s *TestGatewayManagementServer) GetLLM(ctx context.Context, req *gwmgmtpb.GetLLMRequest) (*gwmgmtpb.GetLLMResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "GetLLM",
		Request:   req,
		Timestamp: time.Now(),
	})

	llm, exists := s.llms[req.LlmId]
	if !exists {
		return &gwmgmtpb.GetLLMResponse{}, nil
	}

	return &gwmgmtpb.GetLLMResponse{
		Llm: llm,
	}, nil
}

// ListApps implements the ListApps RPC.
func (s *TestGatewayManagementServer) ListApps(ctx context.Context, req *gwmgmtpb.ListAppsRequest) (*gwmgmtpb.ListAppsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "ListApps",
		Request:   req,
		Timestamp: time.Now(),
	})

	var apps []*gwmgmtpb.AppInfo
	for _, app := range s.apps {
		apps = append(apps, app)
	}

	return &gwmgmtpb.ListAppsResponse{
		Apps:       apps,
		TotalCount: int64(len(apps)),
	}, nil
}

// GetApp implements the GetApp RPC.
func (s *TestGatewayManagementServer) GetApp(ctx context.Context, req *gwmgmtpb.GetAppRequest) (*gwmgmtpb.GetAppResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "GetApp",
		Request:   req,
		Timestamp: time.Now(),
	})

	app, exists := s.apps[req.AppId]
	if !exists {
		return &gwmgmtpb.GetAppResponse{}, nil
	}

	return &gwmgmtpb.GetAppResponse{
		App: app,
	}, nil
}

// GetBudgetStatus implements the GetBudgetStatus RPC.
func (s *TestGatewayManagementServer) GetBudgetStatus(ctx context.Context, req *gwmgmtpb.GetBudgetStatusRequest) (*gwmgmtpb.GetBudgetStatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "GetBudgetStatus",
		Request:   req,
		Timestamp: time.Now(),
	})

	status, exists := s.budgetStatus[req.AppId]
	if !exists {
		// Return default budget status
		return &gwmgmtpb.GetBudgetStatusResponse{
			AppId:           req.AppId,
			MonthlyBudget:   1000.0,
			CurrentUsage:    0.0,
			RemainingBudget: 1000.0,
			IsOverBudget:    false,
			PercentageUsed:  0.0,
		}, nil
	}

	return status, nil
}

// ListModelPrices implements the ListModelPrices RPC.
func (s *TestGatewayManagementServer) ListModelPrices(ctx context.Context, req *gwmgmtpb.ListModelPricesRequest) (*gwmgmtpb.ListModelPricesResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "ListModelPrices",
		Request:   req,
		Timestamp: time.Now(),
	})

	return &gwmgmtpb.ListModelPricesResponse{
		ModelPrices: s.modelPrices,
	}, nil
}

// GetModelPrice implements the GetModelPrice RPC.
func (s *TestGatewayManagementServer) GetModelPrice(ctx context.Context, req *gwmgmtpb.GetModelPriceRequest) (*gwmgmtpb.GetModelPriceResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "GetModelPrice",
		Request:   req,
		Timestamp: time.Now(),
	})

	for _, price := range s.modelPrices {
		if price.ModelName == req.ModelName && price.Vendor == req.Vendor {
			return &gwmgmtpb.GetModelPriceResponse{
				ModelPrice: price,
			}, nil
		}
	}

	return &gwmgmtpb.GetModelPriceResponse{}, nil
}

// ValidateCredential implements the ValidateCredential RPC.
func (s *TestGatewayManagementServer) ValidateCredential(ctx context.Context, req *gwmgmtpb.ValidateCredentialRequest) (*gwmgmtpb.ValidateCredentialResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "ValidateCredential",
		Request:   req,
		Timestamp: time.Now(),
	})

	// For testing, always return valid
	return &gwmgmtpb.ValidateCredentialResponse{
		Valid: true,
	}, nil
}

// QueueControlPayload implements the QueueControlPayload RPC.
func (s *TestGatewayManagementServer) QueueControlPayload(ctx context.Context, req *gwmgmtpb.QueueControlPayloadRequest) (*gwmgmtpb.QueueControlPayloadResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls = append(s.calls, ServiceCall{
		Method:    "QueueControlPayload",
		Request:   req,
		Timestamp: time.Now(),
	})

	s.controlPayloads = append(s.controlPayloads, ControlPayload{
		CorrelationID: req.CorrelationId,
		Payload:       req.Payload,
		Metadata:      req.Metadata,
		Timestamp:     time.Now(),
	})

	return &gwmgmtpb.QueueControlPayloadResponse{
		Success: true,
	}, nil
}

// ============================================================================
// Test Gateway Server Configuration Methods
// ============================================================================

// AddTestApp adds a test app to the gateway server.
func (s *TestGatewayManagementServer) AddTestApp(app *gwmgmtpb.AppInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apps[app.Id] = app
}

// AddTestLLM adds a test LLM to the gateway server.
func (s *TestGatewayManagementServer) AddTestLLM(llm *gwmgmtpb.LLMInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.llms[llm.Id] = llm
}

// SetBudgetStatus sets the budget status for an app.
func (s *TestGatewayManagementServer) SetBudgetStatus(appID uint32, status *gwmgmtpb.GetBudgetStatusResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.budgetStatus[appID] = status
}

// AddModelPrice adds a model price entry.
func (s *TestGatewayManagementServer) AddModelPrice(price *gwmgmtpb.ModelPriceInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelPrices = append(s.modelPrices, price)
}

// GetControlPayloads returns all queued control payloads.
func (s *TestGatewayManagementServer) GetControlPayloads() []ControlPayload {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ControlPayload, len(s.controlPayloads))
	copy(result, s.controlPayloads)
	return result
}

// GetCalls returns all tracked service calls.
func (s *TestGatewayManagementServer) GetCalls() []ServiceCall {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ServiceCall, len(s.calls))
	copy(result, s.calls)
	return result
}

// Reset clears all tracking data.
func (s *TestGatewayManagementServer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.licenseChecked = false
	s.kvStore = make(map[string][]byte)
	s.kvWrites = []KVWrite{}
	s.calls = []ServiceCall{}
	s.apps = make(map[uint32]*gwmgmtpb.AppInfo)
	s.llms = make(map[uint32]*gwmgmtpb.LLMInfo)
	s.budgetStatus = make(map[uint32]*gwmgmtpb.GetBudgetStatusResponse)
	s.modelPrices = []*gwmgmtpb.ModelPriceInfo{}
	s.controlPayloads = []ControlPayload{}
}

// ============================================================================
// Helpers
// ============================================================================

var eventIDCounter int64

func generateEventID() string {
	id := atomic.AddInt64(&eventIDCounter, 1)
	return fmt.Sprintf("test-event-%d", id)
}
