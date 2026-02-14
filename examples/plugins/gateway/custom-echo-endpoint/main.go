package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"google.golang.org/grpc"
)

// Embed UI assets and manifest into the binary
//
//go:embed ui assets manifest.json config.schema.json
var embeddedAssets embed.FS

//go:embed manifest.json
var manifestFile []byte

//go:embed config.schema.json
var configSchemaFile []byte

const (
	PluginName    = "custom-echo-endpoint"
	PluginVersion = "1.0.0"
	PluginDesc    = "Gateway custom endpoint that echoes request metadata with configurable content"

	DefaultContent = "Hello from custom-echo-endpoint! Edit this in the Studio UI."
	DefaultSlug    = "custom-echo-endpoint"
)

// CustomEchoEndpointPlugin demonstrates combining CustomEndpointHandler + UIProvider + ConfigProvider
//
//   - CustomEndpointHandler: serves a catch-all HTTP endpoint on the gateway that echoes back
//     request metadata along with user-configured custom content.
//   - UIProvider: provides a WebComponent UI in the AI Studio admin sidebar for editing the
//     custom content string.
//   - ConfigProvider: provides a JSON Schema so the admin UI can render a configuration form.
//
// Config flow:
//  1. User edits content in the Studio UI → save_content RPC
//  2. RPC handler persists to plugin config in DB via ai_studio_sdk.UpdatePluginConfig()
//  3. Config change syncs to connected gateways via gRPC ConfigurationSnapshot
//  4. Gateway reloads plugin → Initialize() reads updated config["custom_content"]
//  5. Custom endpoint immediately serves the new content
type CustomEchoEndpointPlugin struct {
	plugin_sdk.BasePlugin
	customContent string
	slug          string
}

// NewCustomEchoEndpointPlugin creates a new plugin instance
func NewCustomEchoEndpointPlugin() *CustomEchoEndpointPlugin {
	return &CustomEchoEndpointPlugin{
		BasePlugin:    plugin_sdk.NewBasePlugin(PluginName, PluginVersion, PluginDesc),
		customContent: DefaultContent,
		slug:          DefaultSlug,
	}
}

// ============================================================================
// Plugin lifecycle
// ============================================================================

// Initialize reads custom_content from the config map. On the gateway, this value comes
// from the plugin's config JSON field in the database, synced via gRPC ConfigurationSnapshot.
// On Studio, it's the same config field passed during plugin initialization.
func (p *CustomEchoEndpointPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	log.Printf("%s: Initialized in %s runtime", PluginName, ctx.Runtime)

	if slug, ok := config["slug"]; ok && slug != "" {
		p.slug = slug
		log.Printf("%s: Slug loaded: %q", PluginName, slug)
	} else {
		p.slug = DefaultSlug
		log.Printf("%s: No slug in config, using default: %q", PluginName, DefaultSlug)
	}

	if content, ok := config["custom_content"]; ok && content != "" {
		p.customContent = content
		log.Printf("%s: Custom content loaded: %q", PluginName, content)
	} else {
		p.customContent = DefaultContent
		log.Printf("%s: No custom_content in config, using default", PluginName)
	}

	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *CustomEchoEndpointPlugin) Shutdown(ctx plugin_sdk.Context) error {
	log.Printf("%s: Shutting down", PluginName)
	return nil
}

// ============================================================================
// CustomEndpointHandler — serves custom HTTP endpoints on the gateway
// ============================================================================

// GetEndpointRegistrations declares a catch-all endpoint accepting GET and POST.
// Requests to /plugins/{slug}/* on the gateway will route here.
func (p *CustomEchoEndpointPlugin) GetEndpointRegistrations() ([]*pb.EndpointRegistration, error) {
	return []*pb.EndpointRegistration{
		{
			Path:           "/*",
			Methods:        []string{"GET", "POST"},
			Description:    "Echo endpoint — returns request metadata and custom content",
			RequireAuth:    false,
			StreamResponse: false,
		},
	}, nil
}

// HandleEndpointRequest echoes back the full request metadata alongside the configured
// custom content. This is the core demonstration: the content configured in the Studio UI
// appears in every response from this gateway endpoint.
func (p *CustomEchoEndpointPlugin) HandleEndpointRequest(ctx plugin_sdk.Context, req *pb.EndpointRequest) (*pb.EndpointResponse, error) {
	log.Printf("%s: Endpoint hit — %s %s", PluginName, req.Method, req.RelativePath)

	echo := map[string]interface{}{
		"plugin":         PluginName,
		"version":        PluginVersion,
		"custom_content": p.customContent,
		"request": map[string]interface{}{
			"method":        req.Method,
			"path":          req.Path,
			"relative_path": req.RelativePath,
			"path_segments": req.PathSegments,
			"query_string":  req.QueryString,
			"remote_addr":   req.RemoteAddr,
			"host":          req.Host,
			"headers":       req.Headers,
			"body":          string(req.Body),
			"authenticated": req.Authenticated,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	// Include app info when the request was authenticated
	if req.Authenticated && req.App != nil {
		echo["app"] = map[string]interface{}{
			"id":       req.App.Id,
			"name":     req.App.Name,
			"metadata": req.App.Metadata,
		}
	}

	body, err := json.MarshalIndent(echo, "", "  ")
	if err != nil {
		return &pb.EndpointResponse{
			StatusCode: 500,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(`{"error":"failed to marshal response"}`),
		}, nil
	}

	return &pb.EndpointResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type":            "application/json",
			"X-Plugin-Name":           PluginName,
			"X-Plugin-Custom-Content": p.customContent,
		},
		Body: body,
	}, nil
}

// HandleEndpointRequestStream is not used (StreamResponse is false), but must be
// implemented to satisfy the CustomEndpointHandler interface.
func (p *CustomEchoEndpointPlugin) HandleEndpointRequestStream(
	ctx plugin_sdk.Context,
	req *pb.EndpointRequest,
	stream grpc.ServerStreamingServer[pb.EndpointResponseChunk],
) error {
	return fmt.Errorf("streaming not supported by %s", PluginName)
}

// ============================================================================
// UIProvider — serves the Studio admin UI for editing custom content
// ============================================================================

// GetManifest returns the plugin manifest declaring sidebar UI slots.
func (p *CustomEchoEndpointPlugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// GetAsset serves embedded static assets (JS, CSS, SVG) to the Studio admin UI.
func (p *CustomEchoEndpointPlugin) GetAsset(assetPath string) ([]byte, string, error) {
	if strings.HasPrefix(assetPath, "/") {
		assetPath = strings.TrimPrefix(assetPath, "/")
	}

	content, err := embeddedAssets.ReadFile(assetPath)
	if err != nil {
		return nil, "", fmt.Errorf("asset not found: %s", assetPath)
	}

	mimeType := "application/octet-stream"
	if strings.HasSuffix(assetPath, ".js") {
		mimeType = "application/javascript"
	} else if strings.HasSuffix(assetPath, ".css") {
		mimeType = "text/css"
	} else if strings.HasSuffix(assetPath, ".svg") {
		mimeType = "image/svg+xml"
	} else if strings.HasSuffix(assetPath, ".json") {
		mimeType = "application/json"
	}

	return content, mimeType, nil
}

// ListAssets returns an empty list (not used in this example).
func (p *CustomEchoEndpointPlugin) ListAssets(pathPrefix string) ([]*pb.AssetInfo, error) {
	return []*pb.AssetInfo{}, nil
}

// HandleRPC processes custom RPC method calls from the WebComponent UI.
// Supported methods:
//   - get_config: returns the current slug and custom content
//   - save_config: updates slug + content in memory + persists to plugin config in DB
func (p *CustomEchoEndpointPlugin) HandleRPC(method string, payload []byte) ([]byte, error) {
	log.Printf("%s: RPC call — method: %s", PluginName, method)

	var result interface{}
	var err error

	switch method {
	case "get_config":
		result, err = p.rpcGetConfig(payload)
	case "save_config":
		result, err = p.rpcSaveConfig(payload)
	default:
		return nil, fmt.Errorf("unknown RPC method: %s", method)
	}

	if err != nil {
		log.Printf("%s: RPC error — method: %s, error: %v", PluginName, method, err)
		return nil, err
	}

	return json.Marshal(result)
}

// --- RPC: get_config ---

type getConfigResponse struct {
	Slug    string `json:"slug"`
	Content string `json:"content"`
}

func (p *CustomEchoEndpointPlugin) rpcGetConfig(_ []byte) (*getConfigResponse, error) {
	return &getConfigResponse{
		Slug:    p.slug,
		Content: p.customContent,
	}, nil
}

// --- RPC: save_config ---

type saveConfigRequest struct {
	Slug    string `json:"slug"`
	Content string `json:"content"`
}

type saveConfigResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func (p *CustomEchoEndpointPlugin) rpcSaveConfig(payload []byte) (*saveConfigResponse, error) {
	var req saveConfigRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	if req.Slug == "" {
		return nil, fmt.Errorf("slug is required")
	}

	// Update in-memory state immediately (Studio-side)
	p.slug = req.Slug
	p.customContent = req.Content
	log.Printf("%s: Config updated in memory — slug: %q, content: %q", PluginName, req.Slug, req.Content)

	// Persist to plugin config in the DB via the Studio Service API.
	// This triggers a config sync to connected gateways so the endpoint
	// will serve the new content after the gateway reloads the plugin.
	pluginID := ai_studio_sdk.GetPluginID()
	if pluginID == 0 {
		log.Printf("%s: Plugin ID not set — config will not persist to DB", PluginName)
		return &saveConfigResponse{
			Success: true,
			Message: "Config updated in memory. Plugin ID not available — changes won't persist to gateways.",
		}, nil
	}

	configJSON, _ := json.Marshal(map[string]string{
		"slug":           req.Slug,
		"custom_content": req.Content,
	})

	ctx := context.Background()
	resp, err := ai_studio_sdk.UpdatePluginConfig(ctx, pluginID, string(configJSON))
	if err != nil {
		log.Printf("%s: Failed to persist config: %v", PluginName, err)
		return &saveConfigResponse{
			Success: true,
			Message: fmt.Sprintf("Config updated in memory but failed to persist to DB: %v", err),
		}, nil
	}

	if resp != nil && !resp.Success {
		return &saveConfigResponse{
			Success: true,
			Message: "Config updated in memory but DB persistence failed: " + resp.Message,
		}, nil
	}

	log.Printf("%s: Config persisted to DB (plugin_id=%d)", PluginName, pluginID)

	return &saveConfigResponse{
		Success: true,
		Message: "Configuration saved. Gateways will pick up the change on next config sync.",
	}, nil
}

// ============================================================================
// ConfigProvider — provides JSON Schema for the plugin configuration
// ============================================================================

// GetConfigSchema returns the JSON Schema for this plugin's configuration.
func (p *CustomEchoEndpointPlugin) GetConfigSchema() ([]byte, error) {
	return configSchemaFile, nil
}

// ============================================================================
// main
// ============================================================================

func main() {
	log.Printf("Starting %s v%s", PluginName, PluginVersion)
	plugin := NewCustomEchoEndpointPlugin()
	plugin_sdk.Serve(plugin)
}
