package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gosimple/slug"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// agentProxyBaseURL is the embedded gateway agent plugins' LLM calls go
// through, so every call is authenticated, filtered, budgeted and recorded
// like any App's. A variable so tests can point it at a fake gateway.
var agentProxyBaseURL = "http://localhost:9090"

// maxDecisionCallBytes bounds a CallDecision response read from the gateway.
const maxDecisionCallBytes = 8 << 20

// agentLLMForPlugin loads the agent configuration of plugin (its App, with
// LLMs and credential) and the LLM it may call. It is the access check every
// agent model call shares: the LLM must be one the agent's App holds.
func (s *AIStudioManagementServer) agentLLMForPlugin(plugin *models.Plugin, llmID uint32) (*models.AgentConfig, *models.LLM, error) {
	var agentConfig models.AgentConfig
	if err := s.service.GetDB().
		Preload("App.LLMs").
		Preload("App.Credential").
		Where("plugin_id = ?", plugin.ID).
		First(&agentConfig).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, status.Errorf(codes.NotFound, "no agent configuration found for plugin")
		}
		log.Error().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to load agent config")
		return nil, nil, status.Errorf(codes.Internal, "failed to load agent configuration")
	}

	allowed := false
	for _, l := range agentConfig.App.LLMs {
		if l.ID == uint(llmID) {
			allowed = true
			break
		}
	}
	if !allowed {
		log.Warn().
			Uint32("llm_id", llmID).
			Uint("app_id", agentConfig.App.ID).
			Msg("LLM not allowed for agent's app")
		return nil, nil, status.Errorf(codes.PermissionDenied, "LLM not allowed for this agent")
	}

	var llm models.LLM
	if err := s.service.GetDB().First(&llm, llmID).Error; err != nil {
		log.Error().Err(err).Uint32("llm_id", llmID).Msg("Failed to load LLM")
		return nil, nil, status.Errorf(codes.Internal, "failed to load LLM")
	}
	if agentConfig.App.Credential.Secret == "" {
		return nil, nil, status.Errorf(codes.FailedPrecondition, "the agent's app has no credential")
	}
	return &agentConfig, &llm, nil
}

// CallDecision asks a decision model through the gateway's /ai/{slug}/v1/decisions
// bridge with the agent App's credential, so the call is governed exactly like
// an App's: access, filters, plugins, budget and analytics.
func (s *AIStudioManagementServer) CallDecision(ctx context.Context, req *pb.CallDecisionRequest) (*pb.CallDecisionResponse, error) {
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeLLMsProxy)
	if err != nil {
		return nil, err
	}
	ctx = SetPluginInContext(ctx, plugin)

	agentConfig, llm, err := s.agentLLMForPlugin(plugin, req.GetLlmId())
	if err != nil {
		return nil, err
	}

	body, err := decisionCallBody(req.GetRequestJson(), llm.DefaultModel)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	proxyURL := fmt.Sprintf("%s/ai/%s/v1/decisions", agentProxyBaseURL, slug.Make(llm.Name))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, proxyURL, bytes.NewReader(body))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create proxy request")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+agentConfig.App.Credential.Secret)

	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("proxy_url", proxyURL).Msg("Failed to call decision proxy")
		return &pb.CallDecisionResponse{Success: false, ErrorMessage: fmt.Sprintf("proxy call failed: %v", err)}, nil
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, maxDecisionCallBytes))
	if err != nil {
		return &pb.CallDecisionResponse{Success: false, ErrorMessage: fmt.Sprintf("failed to read response: %v", err), StatusCode: int32(resp.StatusCode)}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return &pb.CallDecisionResponse{
			Success:      false,
			ErrorMessage: decisionCallError(resp.StatusCode, out),
			StatusCode:   int32(resp.StatusCode),
		}, nil
	}

	log.Debug().
		Uint32("llm_id", req.GetLlmId()).
		Uint("app_id", agentConfig.App.ID).
		Uint("plugin_id", plugin.ID).
		Msg("Agent decision call completed")
	return &pb.CallDecisionResponse{Success: true, ResponseJson: string(out), StatusCode: int32(resp.StatusCode)}, nil
}

// decisionCallBody validates the plugin's request JSON as an OpenAI Decisions
// request and fills in the LLM's default model when the request names none.
// The model is bare: the call is addressed to the LLM's own route.
func decisionCallBody(requestJSON, defaultModel string) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(requestJSON), &fields); err != nil {
		return nil, fmt.Errorf("request_json must be an OpenAI Decisions JSON object: %w", err)
	}
	var req decisions.Request
	if err := json.Unmarshal([]byte(requestJSON), &req); err != nil {
		return nil, fmt.Errorf("request_json must be an OpenAI Decisions JSON object: %w", err)
	}
	if req.Model == "" {
		if defaultModel == "" {
			return nil, errors.New("request_json names no model and the LLM has no default model")
		}
		fields["model"], _ = json.Marshal(defaultModel)
	}
	return json.Marshal(fields)
}

// decisionCallError renders a gateway refusal for the plugin: the OpenAI
// envelope's message when there is one.
func decisionCallError(statusCode int, body []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error.Message != "" {
		return fmt.Sprintf("decision call failed (status %d): %s", statusCode, envelope.Error.Message)
	}
	return fmt.Sprintf("decision call failed (status %d): %s", statusCode, string(bytes.TrimSpace(body)))
}
