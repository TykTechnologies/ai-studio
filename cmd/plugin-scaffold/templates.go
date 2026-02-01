package main

// templates contains all plugin templates keyed by path
var templates = map[string]string{
	"studio/main.go.tmpl":               studioMainTemplate,
	"studio/go.mod.tmpl":                studioGoModTemplate,
	"studio/manifest.json.tmpl":         studioManifestTemplate,
	"studio/config.schema.json.tmpl":    configSchemaTemplate,
	"gateway/main.go.tmpl":              gatewayMainTemplate,
	"gateway/go.mod.tmpl":               gatewayGoModTemplate,
	"gateway/manifest.json.tmpl":        gatewayManifestTemplate,
	"agent/main.go.tmpl":                agentMainTemplate,
	"agent/go.mod.tmpl":                 agentGoModTemplate,
	"agent/plugin.manifest.json.tmpl":   agentManifestTemplate,
	"agent/config.schema.json.tmpl":     agentConfigSchemaTemplate,
	"data-collector/main.go.tmpl":       dataCollectorMainTemplate,
	"data-collector/go.mod.tmpl":        dataCollectorGoModTemplate,
	"data-collector/manifest.json.tmpl": dataCollectorManifestTemplate,
	"ui/dashboard.js.tmpl":              dashboardJSTemplate,
	"common/README.md.tmpl":             readmeTemplate,
}

// iconSVG is the default plugin icon
const iconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
  <path d="M12 2L2 7l10 5 10-5-10-5z"/>
  <path d="M2 17l10 5 10-5"/>
  <path d="M2 12l10 5 10-5"/>
</svg>`

// Studio Plugin Templates

const studioMainTemplate = `package main

import (
	"embed"
{{- if .HasUI}}
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"path/filepath"
	"strings"
{{- else if .HasAuth}}
	"fmt"
	"strings"
{{- end}}
	"log"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

{{if .HasUI -}}
//go:embed ui assets manifest.json config.schema.json
var embeddedAssets embed.FS
{{else -}}
//go:embed manifest.json config.schema.json
var embeddedAssets embed.FS
{{end}}
//go:embed manifest.json
var manifestFile []byte

//go:embed config.schema.json
var configSchemaFile []byte

const (
	PluginName    = "{{.Name}}"
	PluginVersion = "1.0.0"
)

// {{.StructName}}Plugin implements the plugin interface
type {{.StructName}}Plugin struct {
	plugin_sdk.BasePlugin
}

// New{{.StructName}}Plugin creates a new plugin instance
func New{{.StructName}}Plugin() *{{.StructName}}Plugin {
	return &{{.StructName}}Plugin{
		BasePlugin: plugin_sdk.NewBasePlugin(
			PluginName,
			PluginVersion,
			"{{.DisplayName}} Plugin",
		),
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *{{.StructName}}Plugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	log.Printf("%s: Initialized in %s runtime", PluginName, ctx.Runtime)

	// TODO: Parse your configuration here
	// Example: if val, ok := config["my_setting"]; ok { ... }

	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *{{.StructName}}Plugin) Shutdown(ctx plugin_sdk.Context) error {
	log.Printf("%s: Shutting down", PluginName)
	return nil
}

// GetManifest implements plugin_sdk.ManifestProvider
func (p *{{.StructName}}Plugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *{{.StructName}}Plugin) GetConfigSchema() ([]byte, error) {
	return configSchemaFile, nil
}

{{if .HasSessionAware -}}
// OnSessionReady implements plugin_sdk.SessionAware
func (p *{{.StructName}}Plugin) OnSessionReady(ctx plugin_sdk.Context) {
	log.Printf("%s: Session ready", PluginName)
	// TODO: Initialize session-dependent resources here
}

// OnSessionClosing implements plugin_sdk.SessionAware
func (p *{{.StructName}}Plugin) OnSessionClosing(ctx plugin_sdk.Context) {
	log.Printf("%s: Session closing", PluginName)
}

{{end -}}
{{if .HasPreAuth -}}
// HandlePreAuth implements plugin_sdk.PreAuthHandler
func (p *{{.StructName}}Plugin) HandlePreAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	log.Printf("%s: HandlePreAuth called", PluginName)

	// TODO: Add your pre-auth logic here
	// This runs BEFORE authentication

	return &pb.PluginResponse{Modified: false}, nil
}

{{end -}}
{{if .HasAuth -}}
// HandleAuth implements plugin_sdk.AuthHandler
func (p *{{.StructName}}Plugin) HandleAuth(ctx plugin_sdk.Context, req *pb.AuthRequest) (*pb.AuthResponse, error) {
	log.Printf("%s: HandleAuth called", PluginName)

	// Extract credential (handles Bearer prefix)
	credential := req.Credential
	if strings.HasPrefix(credential, "Bearer ") {
		credential = strings.TrimPrefix(credential, "Bearer ")
	}

	// Access request headers if needed: req.Request.Headers

	// TODO: Add your authentication logic here
	// Return Authenticated: false with ErrorMessage to reject
	// See examples/plugins/studio/custom-auth-ui for full implementation

	return &pb.AuthResponse{
		Authenticated: true,
		UserId:        "user-id",
		AppId:         "1",
		Claims: map[string]string{
			"source": PluginName,
		},
	}, nil
}

// GetAppByCredential implements plugin_sdk.AuthHandler
func (p *{{.StructName}}Plugin) GetAppByCredential(ctx plugin_sdk.Context, credential string) (*pb.App, error) {
	// TODO: Return app for this credential, or error if not found
	// See examples/plugins/studio/custom-auth-ui for runtime-aware implementation
	return nil, fmt.Errorf("app lookup not implemented")
}

// GetUserByCredential implements plugin_sdk.AuthHandler
func (p *{{.StructName}}Plugin) GetUserByCredential(ctx plugin_sdk.Context, credential string) (*pb.User, error) {
	// TODO: Return user for this credential, or error if not found
	return nil, fmt.Errorf("user lookup not implemented")
}

{{end -}}
{{if .HasPostAuth -}}
// HandlePostAuth implements plugin_sdk.PostAuthHandler
func (p *{{.StructName}}Plugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	pluginCtx := req.Request.Context
	log.Printf("%s: HandlePostAuth called for app %d", PluginName, pluginCtx.AppId)

	// TODO: Add your post-auth logic here
	// To block a request:
	//   return &pb.PluginResponse{Block: true, StatusCode: 403, Body: []byte(` + "`" + `{"error":"blocked"}` + "`" + `)}, nil
	// To modify the request:
	//   return &pb.PluginResponse{Modified: true, Body: modifiedBody}, nil

	return &pb.PluginResponse{Modified: false}, nil
}

{{end -}}
{{if .HasOnResponse -}}
// OnBeforeWriteHeaders implements plugin_sdk.ResponseHandler
func (p *{{.StructName}}Plugin) OnBeforeWriteHeaders(ctx plugin_sdk.Context, req *pb.HeadersRequest) (*pb.HeadersResponse, error) {
	// TODO: Modify response headers here
	return &pb.HeadersResponse{
		Modified: false,
		Headers:  req.Headers,
	}, nil
}

// OnBeforeWrite implements plugin_sdk.ResponseHandler
func (p *{{.StructName}}Plugin) OnBeforeWrite(ctx plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
	// TODO: Modify response body here
	return &pb.ResponseWriteResponse{
		Modified: false,
		Body:     req.Body,
		Headers:  req.Headers,
	}, nil
}

{{end -}}
{{if .HasObjectHooks -}}
// GetObjectHookRegistrations implements plugin_sdk.ObjectHookHandler
func (p *{{.StructName}}Plugin) GetObjectHookRegistrations() ([]*pb.ObjectHookRegistration, error) {
	return []*pb.ObjectHookRegistration{
		{ObjectType: "llm", HookType: "before_create"},
		{ObjectType: "llm", HookType: "after_create"},
	}, nil
}

// HandleObjectHook implements plugin_sdk.ObjectHookHandler
func (p *{{.StructName}}Plugin) HandleObjectHook(ctx plugin_sdk.Context, req *pb.ObjectHookRequest) (*pb.ObjectHookResponse, error) {
	log.Printf("%s: HandleObjectHook - %s %s", PluginName, req.HookType, req.ObjectType)

	// TODO: Add your object hook logic here
	// Return Block: true to prevent the operation

	return &pb.ObjectHookResponse{
		Block:          false,
		ModifiedObject: req.ObjectJson,
	}, nil
}

{{end -}}
{{if .HasUI -}}
// GetAsset implements plugin_sdk.UIProvider
func (p *{{.StructName}}Plugin) GetAsset(path string) ([]byte, string, error) {
	cleanPath := strings.TrimPrefix(path, "/")
	data, err := embeddedAssets.ReadFile(cleanPath)
	if err != nil {
		return nil, "", fmt.Errorf("asset not found: %s", path)
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return data, contentType, nil
}

// ListAssets implements plugin_sdk.UIProvider
func (p *{{.StructName}}Plugin) ListAssets(prefix string) ([]*pb.AssetInfo, error) {
	var assets []*pb.AssetInfo
	err := fs.WalkDir(embeddedAssets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			assets = append(assets, &pb.AssetInfo{Path: "/" + path})
		}
		return nil
	})
	return assets, err
}

// HandleRPC implements plugin_sdk.RPCHandler
func (p *{{.StructName}}Plugin) HandleRPC(method string, payload []byte) ([]byte, error) {
	log.Printf("%s: HandleRPC - method: %s", PluginName, method)

	switch method {
	case "getData":
		// TODO: Implement your RPC methods
		return json.Marshal(map[string]interface{}{
			"status": "ok",
			"data":   []interface{}{},
		})

	case "saveData":
		// TODO: Parse payload and save data
		var input map[string]interface{}
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, fmt.Errorf("invalid payload: %w", err)
		}
		return json.Marshal(map[string]interface{}{"success": true})

	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

{{end -}}
func main() {
	log.Printf("Starting %s Plugin v%s", PluginName, PluginVersion)
	plugin := New{{.StructName}}Plugin()
	plugin_sdk.Serve(plugin)
}
`

const studioGoModTemplate = `module github.com/TykTechnologies/midsommar/v2/examples/plugins/studio/{{.Name}}

go 1.24.10

replace github.com/TykTechnologies/midsommar/v2 => {{.RelativeReplace}}

replace github.com/TykTechnologies/midsommar/microgateway => {{.RelativeReplace}}/microgateway

require github.com/TykTechnologies/midsommar/v2 v2.0.0
`

const studioManifestTemplate = `{
  "id": "com.tyk.{{.Name}}",
  "version": "1.0.0",
  "name": "{{.DisplayName}}",
  "description": "A Tyk AI Studio plugin",
  "capabilities": {
    "hooks": [{{range $i, $cap := .Capabilities}}{{if $i}}, {{end}}"{{$cap}}"{{end}}],
    "primary_hook": "{{.PrimaryHook}}"
  },
  "permissions": {
    "services": ["kv.readwrite"]{{if .HasUI}},
    "ui": ["sidebar.register", "route.register"],
    "rpc": ["call"]{{end}}
  }{{if .HasUI}},
  "ui": {
    "slots": [
      {
        "slot": "sidebar.section",
        "label": "{{.DisplayName}}",
        "icon": "/assets/icon.svg",
        "items": [
          {
            "type": "route",
            "path": "/admin/plugins/{{.Name}}",
            "title": "Dashboard",
            "mount": {
              "kind": "webc",
              "tag": "{{.Name}}-dashboard",
              "entry": "/ui/webc/dashboard.js"
            }
          }
        ]
      }
    ]
  },
  "assets": [
    "/assets/icon.svg",
    "/ui/webc/dashboard.js"
  ]{{end}},
  "compat": {
    "min_studio_version": "2.6.0",
    "min_gateway_version": "1.0.0"
  },
  "security": {
    "csp": "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'"
  }
}
`

const configSchemaTemplate = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "title": "{{.DisplayName}} Configuration",
  "description": "Configuration options for {{.Name}}",
  "properties": {
    "enabled": {
      "type": "boolean",
      "title": "Enabled",
      "description": "Whether the plugin is enabled",
      "default": true
    }
  },
  "additionalProperties": false
}
`

// Gateway Plugin Templates

const gatewayMainTemplate = `package main

import (
	_ "embed"
	"encoding/json"
{{- if .HasAuth}}
	"fmt"
{{- end}}
	"log"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

//go:embed manifest.json
var manifestBytes []byte

const (
	PluginName    = "{{.Name}}"
	PluginVersion = "1.0.0"
)

// {{.StructName}}Plugin implements the gateway plugin interface
type {{.StructName}}Plugin struct {
	plugin_sdk.BasePlugin
}

// New{{.StructName}}Plugin creates a new plugin instance
func New{{.StructName}}Plugin() *{{.StructName}}Plugin {
	return &{{.StructName}}Plugin{
		BasePlugin: plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "{{.DisplayName}}"),
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *{{.StructName}}Plugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	log.Printf("%s: Initialized", PluginName)
	// TODO: Parse configuration here
	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *{{.StructName}}Plugin) Shutdown(ctx plugin_sdk.Context) error {
	return nil
}

// GetManifest implements plugin_sdk.ManifestProvider
func (p *{{.StructName}}Plugin) GetManifest() ([]byte, error) {
	return manifestBytes, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *{{.StructName}}Plugin) GetConfigSchema() ([]byte, error) {
	schema := map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"type":        "object",
		"title":       "{{.DisplayName}} Configuration",
		"properties":  map[string]interface{}{},
	}
	return json.Marshal(schema)
}

{{if .HasPreAuth -}}
// HandlePreAuth implements plugin_sdk.PreAuthHandler
func (p *{{.StructName}}Plugin) HandlePreAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	// TODO: Add pre-auth logic
	return &pb.PluginResponse{Modified: false}, nil
}

{{end -}}
{{if .HasAuth -}}
// HandleAuth implements plugin_sdk.AuthHandler
func (p *{{.StructName}}Plugin) HandleAuth(ctx plugin_sdk.Context, req *pb.AuthRequest) (*pb.AuthResponse, error) {
	log.Printf("%s: HandleAuth called", PluginName)

	// TODO: Add authentication logic
	// Return Authenticated: false with ErrorMessage to reject
	// See examples/plugins/studio/custom-auth-ui for full implementation

	return &pb.AuthResponse{
		Authenticated: true,
		UserId:        "user-id",
		AppId:         "1",
		Claims: map[string]string{
			"source": PluginName,
		},
	}, nil
}

// GetAppByCredential implements plugin_sdk.AuthHandler
func (p *{{.StructName}}Plugin) GetAppByCredential(ctx plugin_sdk.Context, credential string) (*pb.App, error) {
	// TODO: Return app for this credential
	return nil, fmt.Errorf("app lookup not implemented")
}

// GetUserByCredential implements plugin_sdk.AuthHandler
func (p *{{.StructName}}Plugin) GetUserByCredential(ctx plugin_sdk.Context, credential string) (*pb.User, error) {
	// TODO: Return user for this credential
	return nil, fmt.Errorf("user lookup not implemented")
}

{{end -}}
{{if .HasPostAuth -}}
// HandlePostAuth implements plugin_sdk.PostAuthHandler
func (p *{{.StructName}}Plugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	log.Printf("%s: HandlePostAuth for app %d", PluginName, req.Request.Context.AppId)

	// TODO: Add your post-auth logic here
	// To block: return &pb.PluginResponse{Block: true, StatusCode: 403}, nil
	// To modify: return &pb.PluginResponse{Modified: true, Body: modified}, nil

	return &pb.PluginResponse{Modified: false}, nil
}

{{end -}}
{{if .HasOnResponse -}}
// OnBeforeWriteHeaders implements plugin_sdk.ResponseHandler
func (p *{{.StructName}}Plugin) OnBeforeWriteHeaders(ctx plugin_sdk.Context, req *pb.HeadersRequest) (*pb.HeadersResponse, error) {
	return &pb.HeadersResponse{Modified: false, Headers: req.Headers}, nil
}

// OnBeforeWrite implements plugin_sdk.ResponseHandler
func (p *{{.StructName}}Plugin) OnBeforeWrite(ctx plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
	return &pb.ResponseWriteResponse{Modified: false, Body: req.Body, Headers: req.Headers}, nil
}

{{end -}}
func main() {
	plugin := New{{.StructName}}Plugin()
	plugin_sdk.Serve(plugin)
}
`

const gatewayGoModTemplate = `module github.com/TykTechnologies/midsommar/examples/plugins/gateway/{{.Name}}

go 1.24.10

replace github.com/TykTechnologies/midsommar/v2 => {{.RelativeReplace}}

replace github.com/TykTechnologies/midsommar/microgateway => {{.RelativeReplace}}/microgateway

require github.com/TykTechnologies/midsommar/v2 v2.0.0
`

const gatewayManifestTemplate = `{
  "name": "{{.Name}}",
  "version": "1.0.0",
  "description": "{{.DisplayName}} gateway plugin",
  "capabilities": [{{range $i, $cap := .Capabilities}}{{if $i}}, {{end}}"{{$cap}}"{{end}}]
}
`

// Agent Plugin Templates

const agentMainTemplate = `package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	mgmt "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

//go:embed plugin.manifest.json
var manifestFile []byte

//go:embed config.schema.json
var configSchemaFile []byte

const (
	PluginName    = "{{.Name}}"
	PluginVersion = "1.0.0"
)

// Config holds the agent configuration
type Config struct {
	DefaultLLMID uint32 ` + "`" + `json:"default_llm_id"` + "`" + `
	Model        string ` + "`" + `json:"model"` + "`" + `
	SystemPrompt string ` + "`" + `json:"system_prompt"` + "`" + `
}

// {{.StructName}}Plugin implements the agent plugin interface
type {{.StructName}}Plugin struct {
	plugin_sdk.BasePlugin
	config Config
}

// New{{.StructName}}Plugin creates a new agent plugin instance
func New{{.StructName}}Plugin() *{{.StructName}}Plugin {
	return &{{.StructName}}Plugin{
		BasePlugin: plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "{{.DisplayName}} Agent"),
		config: Config{
			SystemPrompt: "You are a helpful assistant.",
		},
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *{{.StructName}}Plugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	log.Printf("%s: Initialized in %s runtime", PluginName, ctx.Runtime)

	// Parse configuration
	if sysPrompt, ok := config["system_prompt"]; ok {
		p.config.SystemPrompt = sysPrompt
	}
	if model, ok := config["model"]; ok {
		p.config.Model = model
	}

	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *{{.StructName}}Plugin) Shutdown(ctx plugin_sdk.Context) error {
	log.Printf("%s: Shutdown called", PluginName)
	return nil
}

// OnSessionReady implements plugin_sdk.SessionAware
func (p *{{.StructName}}Plugin) OnSessionReady(ctx plugin_sdk.Context) {
	log.Printf("%s: Session ready, warming up Service API connection", PluginName)

	if ai_studio_sdk.IsInitialized() {
		_, err := ai_studio_sdk.GetPluginsCount(context.Background())
		if err != nil {
			log.Printf("%s: Service API warmup failed: %v", PluginName, err)
		} else {
			log.Printf("%s: Service API connection established", PluginName)
		}
	}
}

// OnSessionClosing implements plugin_sdk.SessionAware
func (p *{{.StructName}}Plugin) OnSessionClosing(ctx plugin_sdk.Context) {
	log.Printf("%s: Session closing", PluginName)
}

// GetManifest implements plugin_sdk.ManifestProvider
func (p *{{.StructName}}Plugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *{{.StructName}}Plugin) GetConfigSchema() ([]byte, error) {
	return configSchemaFile, nil
}

// ListAssets implements plugin_sdk.UIProvider
func (p *{{.StructName}}Plugin) ListAssets(pathPrefix string) ([]*pb.AssetInfo, error) {
	return []*pb.AssetInfo{}, nil
}

// HandleAgentMessage implements plugin_sdk.AgentPlugin
func (p *{{.StructName}}Plugin) HandleAgentMessage(req *pb.AgentMessageRequest, stream pb.PluginService_HandleAgentMessageServer) error {
	log.Printf("%s: Received message: %s", PluginName, req.UserMessage)

	// Set service broker ID if provided
	if req.ServiceBrokerId != 0 {
		ai_studio_sdk.SetServiceBrokerID(req.ServiceBrokerId)
	}

	// Parse config from request if present
	if req.ConfigJson != "" {
		if err := json.Unmarshal([]byte(req.ConfigJson), &p.config); err != nil {
			log.Printf("%s: Failed to parse config: %v", PluginName, err)
		}
	}

	// Select LLM to use
	var selectedLLM *pb.AgentLLMInfo
	if p.config.DefaultLLMID > 0 {
		for _, llm := range req.AvailableLlms {
			if llm.Id == p.config.DefaultLLMID {
				selectedLLM = llm
				break
			}
		}
	}
	if selectedLLM == nil && len(req.AvailableLlms) > 0 {
		selectedLLM = req.AvailableLlms[0]
	}

	if selectedLLM == nil {
		return stream.Send(&pb.AgentMessageChunk{
			Type:    pb.AgentMessageChunk_ERROR,
			Content: "No LLM available",
			IsFinal: true,
		})
	}

	return p.callLLM(req, selectedLLM, stream)
}

func (p *{{.StructName}}Plugin) callLLM(req *pb.AgentMessageRequest, llm *pb.AgentLLMInfo, stream pb.PluginService_HandleAgentMessageServer) error {
	ctx := stream.Context()

	// Build messages
	messages := []*mgmt.LLMMessage{}

	// Add system prompt
	if p.config.SystemPrompt != "" {
		messages = append(messages, &mgmt.LLMMessage{
			Role:    "system",
			Content: p.config.SystemPrompt,
		})
	}

	// Add history
	for _, histMsg := range req.History {
		messages = append(messages, &mgmt.LLMMessage{
			Role:    histMsg.Role,
			Content: histMsg.Content,
		})
	}

	// Add current message
	messages = append(messages, &mgmt.LLMMessage{
		Role:    "user",
		Content: req.UserMessage,
	})

	// Determine model
	model := p.config.Model
	if model == "" {
		model = llm.DefaultModel
	}

	log.Printf("%s: Calling LLM %d with model %s", PluginName, llm.Id, model)

	// Call LLM
	llmStream, err := ai_studio_sdk.CallLLM(ctx, llm.Id, model, messages, 0.7, 1000, nil, true)
	if err != nil {
		return stream.Send(&pb.AgentMessageChunk{
			Type:    pb.AgentMessageChunk_ERROR,
			Content: fmt.Sprintf("Failed to call LLM: %v", err),
			IsFinal: true,
		})
	}

	// Stream response
	for {
		resp, err := llmStream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return stream.Send(&pb.AgentMessageChunk{
				Type:    pb.AgentMessageChunk_ERROR,
				Content: fmt.Sprintf("Error receiving from LLM: %v", err),
				IsFinal: true,
			})
		}

		if !resp.Success {
			return stream.Send(&pb.AgentMessageChunk{
				Type:    pb.AgentMessageChunk_ERROR,
				Content: resp.ErrorMessage,
				IsFinal: true,
			})
		}

		if resp.Content != "" {
			if err := stream.Send(&pb.AgentMessageChunk{
				Type:    pb.AgentMessageChunk_CONTENT,
				Content: resp.Content,
				IsFinal: false,
			}); err != nil {
				return err
			}
		}

		if resp.Done {
			break
		}
	}

	return stream.Send(&pb.AgentMessageChunk{
		Type:    pb.AgentMessageChunk_DONE,
		Content: "completed",
		IsFinal: true,
	})
}

func main() {
	log.Printf("Starting %s Agent v%s", PluginName, PluginVersion)
	plugin := New{{.StructName}}Plugin()
	plugin_sdk.Serve(plugin)
}
`

const agentGoModTemplate = `module github.com/TykTechnologies/midsommar/v2/examples/plugins/studio/{{.Name}}/server

go 1.24.10

replace github.com/TykTechnologies/midsommar/v2 => {{.RelativeReplace}}

replace github.com/TykTechnologies/midsommar/microgateway => {{.RelativeReplace}}/microgateway

require github.com/TykTechnologies/midsommar/v2 v2.0.0
`

const agentManifestTemplate = `{
  "id": "com.tyk.{{.Name}}",
  "version": "1.0.0",
  "name": "{{.DisplayName}}",
  "description": "{{.DisplayName}} - A conversational AI agent",
  "capabilities": {
    "hooks": ["agent"],
    "primary_hook": "agent"
  },
  "permissions": {
    "services": ["kv.readwrite", "llms.proxy"]
  },
  "compat": {
    "min_studio_version": "2.6.0"
  }
}
`

const agentConfigSchemaTemplate = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "title": "{{.DisplayName}} Configuration",
  "description": "Configuration options for {{.Name}} agent",
  "properties": {
    "default_llm_id": {
      "type": "integer",
      "title": "Default LLM ID",
      "description": "ID of the LLM to use (0 = use first available)"
    },
    "model": {
      "type": "string",
      "title": "Model",
      "description": "Model to use (empty = use LLM default)"
    },
    "system_prompt": {
      "type": "string",
      "title": "System Prompt",
      "description": "System prompt for the agent",
      "default": "You are a helpful assistant."
    }
  },
  "additionalProperties": false
}
`

// Data Collector Plugin Templates

const dataCollectorMainTemplate = `package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

//go:embed manifest.json
var manifestBytes []byte

const (
	PluginName    = "{{.Name}}"
	PluginVersion = "1.0.0"
)

// {{.StructName}}Plugin implements the data collector interface
type {{.StructName}}Plugin struct {
	plugin_sdk.BasePlugin
	outputDir string
	enabled   bool
}

// New{{.StructName}}Plugin creates a new data collector plugin
func New{{.StructName}}Plugin() *{{.StructName}}Plugin {
	return &{{.StructName}}Plugin{
		BasePlugin: plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "{{.DisplayName}}"),
		outputDir:  "./data",
		enabled:    true,
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *{{.StructName}}Plugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	log.Printf("%s: Initialized", PluginName)

	if dir, ok := config["output_dir"]; ok {
		p.outputDir = dir
	}
	if enabled, ok := config["enabled"]; ok {
		p.enabled = enabled == "true"
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(p.outputDir, 0755); err != nil {
		log.Printf("%s: Warning - could not create output dir: %v", PluginName, err)
	}

	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *{{.StructName}}Plugin) Shutdown(ctx plugin_sdk.Context) error {
	return nil
}

// GetManifest implements plugin_sdk.ManifestProvider
func (p *{{.StructName}}Plugin) GetManifest() ([]byte, error) {
	return manifestBytes, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *{{.StructName}}Plugin) GetConfigSchema() ([]byte, error) {
	schema := map[string]interface{}{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"title":   "{{.DisplayName}} Configuration",
		"properties": map[string]interface{}{
			"output_dir": map[string]interface{}{
				"type":        "string",
				"title":       "Output Directory",
				"description": "Directory to write collected data",
				"default":     "./data",
			},
			"enabled": map[string]interface{}{
				"type":        "boolean",
				"title":       "Enabled",
				"description": "Whether data collection is enabled",
				"default":     true,
			},
		},
	}
	return json.Marshal(schema)
}

// HandleProxyLog implements plugin_sdk.DataCollector
func (p *{{.StructName}}Plugin) HandleProxyLog(ctx plugin_sdk.Context, req *pb.ProxyLogRequest) (*pb.DataCollectionResponse, error) {
	if !p.enabled {
		return &pb.DataCollectionResponse{Success: true, Handled: false}, nil
	}

	log.Printf("%s: Received proxy log", PluginName)

	// TODO: Process proxy log data
	// req.AppId, req.UserId, req.LlmId, req.RequestTokens, req.ResponseTokens, etc.

	return &pb.DataCollectionResponse{
		Success: true,
		Handled: true,
	}, nil
}

// HandleAnalytics implements plugin_sdk.DataCollector
func (p *{{.StructName}}Plugin) HandleAnalytics(ctx plugin_sdk.Context, req *pb.AnalyticsRequest) (*pb.DataCollectionResponse, error) {
	if !p.enabled {
		return &pb.DataCollectionResponse{Success: true, Handled: false}, nil
	}

	log.Printf("%s: Received analytics data", PluginName)

	// TODO: Process analytics data

	return &pb.DataCollectionResponse{
		Success: true,
		Handled: true,
	}, nil
}

// HandleBudgetUsage implements plugin_sdk.DataCollector
func (p *{{.StructName}}Plugin) HandleBudgetUsage(ctx plugin_sdk.Context, req *pb.BudgetUsageRequest) (*pb.DataCollectionResponse, error) {
	if !p.enabled {
		return &pb.DataCollectionResponse{Success: true, Handled: false}, nil
	}

	log.Printf("%s: Received budget usage data", PluginName)

	// TODO: Process budget usage data
	// req.UserId, req.AppId, req.TokensUsed, req.CostUsd, etc.

	return &pb.DataCollectionResponse{
		Success: true,
		Handled: true,
	}, nil
}

// writeToFile is a helper to write JSON data to a file
func (p *{{.StructName}}Plugin) writeToFile(prefix string, data interface{}) error {
	timestamp := time.Now().Format("20060102-150405")
	filename := filepath.Join(p.outputDir, prefix+"-"+timestamp+".json")

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, jsonData, 0644)
}

func main() {
	plugin := New{{.StructName}}Plugin()
	plugin_sdk.Serve(plugin)
}
`

const dataCollectorGoModTemplate = `module github.com/TykTechnologies/midsommar/v2/examples/plugins/data-collectors/{{.Name}}

go 1.24.10

replace github.com/TykTechnologies/midsommar/v2 => {{.RelativeReplace}}

replace github.com/TykTechnologies/midsommar/microgateway => {{.RelativeReplace}}/microgateway

require github.com/TykTechnologies/midsommar/v2 v2.0.0
`

const dataCollectorManifestTemplate = `{
  "name": "{{.Name}}",
  "version": "1.0.0",
  "description": "{{.DisplayName}} - Data collection plugin",
  "capabilities": ["data_collector"]
}
`

// UI Templates

const dashboardJSTemplate = `// {{.DisplayName}} Dashboard Web Component
class {{.StructName}}Dashboard extends HTMLElement {
  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.data = {
      items: [],
      loading: true,
      error: null
    };
  }

  connectedCallback() {
    console.log('{{.StructName}}Dashboard component initialized');
    this.render();
    this.waitForPluginAPI();
  }

  async waitForPluginAPI() {
    for (let i = 0; i < 50; i++) {
      if (this.pluginAPI) {
        console.log('Plugin API found, loading data...');
        this.loadData();
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    console.error('Plugin API injection timeout');
    this.showError('Plugin API initialization timeout - please refresh the page');
  }

  async loadData() {
    this.setLoading(true);
    try {
      const result = await this.pluginAPI.call('getData', {});
      console.log('Data loaded:', result);
      this.data.items = result.data || [];
      this.updateContent();
    } catch (error) {
      console.error('Failed to load data:', error);
      this.showError('Failed to load data: ' + error.message);
    } finally {
      this.setLoading(false);
    }
  }

  setLoading(loading) {
    this.data.loading = loading;
    const loadingEl = this.shadowRoot.querySelector('#loading');
    const contentEl = this.shadowRoot.querySelector('#content');
    if (loadingEl) loadingEl.style.display = loading ? 'block' : 'none';
    if (contentEl) contentEl.style.display = loading ? 'none' : 'block';
  }

  showError(message) {
    this.data.error = message;
    const errorEl = this.shadowRoot.querySelector('#error');
    if (errorEl) {
      errorEl.textContent = message;
      errorEl.style.display = 'block';
    }
  }

  updateContent() {
    const contentEl = this.shadowRoot.querySelector('#items-container');
    if (!contentEl) return;

    if (this.data.items.length === 0) {
      contentEl.innerHTML = '<p>No items found.</p>';
      return;
    }

    contentEl.innerHTML = this.data.items.map(item => ` + "`" + `
      <div class="item">
        <span>${JSON.stringify(item)}</span>
      </div>
    ` + "`" + `).join('');
  }

  render() {
    this.shadowRoot.innerHTML = ` + "`" + `
      <style>
        :host {
          display: block;
          font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
          padding: 20px;
        }
        .header {
          display: flex;
          justify-content: space-between;
          align-items: center;
          margin-bottom: 20px;
        }
        h1 {
          margin: 0;
          font-size: 24px;
          color: #333;
        }
        #loading {
          text-align: center;
          padding: 40px;
          color: #666;
        }
        #error {
          display: none;
          padding: 12px;
          background: #fee;
          border: 1px solid #fcc;
          border-radius: 4px;
          color: #c00;
          margin-bottom: 20px;
        }
        .item {
          padding: 12px;
          border: 1px solid #e0e0e0;
          border-radius: 4px;
          margin-bottom: 8px;
          background: #fafafa;
        }
        button {
          padding: 8px 16px;
          background: #007bff;
          color: white;
          border: none;
          border-radius: 4px;
          cursor: pointer;
        }
        button:hover {
          background: #0056b3;
        }
      </style>

      <div class="header">
        <h1>{{.DisplayName}}</h1>
        <button id="refresh-btn">Refresh</button>
      </div>

      <div id="error"></div>

      <div id="loading">Loading...</div>

      <div id="content" style="display: none;">
        <div id="items-container"></div>
      </div>
    ` + "`" + `;

    this.shadowRoot.querySelector('#refresh-btn').addEventListener('click', () => this.loadData());
  }
}

customElements.define('{{.Name}}-dashboard', {{.StructName}}Dashboard);
`

// Common Templates

const readmeTemplate = `# {{.DisplayName}}

A Tyk AI Studio {{.Type}} plugin.

## Capabilities

{{range .Capabilities}}- {{.}}
{{end}}

## Building

` + "```bash" + `
cd {{.OutputDir}}
go build -o {{.Name}}
` + "```" + `

## Registering

1. Start the dev environment: ` + "`make dev-full`" + `
2. Open the Admin UI: http://localhost:3000
3. Go to Admin > Plugins > Register Plugin
4. Use the file path:
   - In Docker: ` + "`file:///app/{{.OutputDir}}/{{.Name}}`" + `
   - Local: ` + "`file://$(pwd)/{{.OutputDir}}/{{.Name}}`" + `

## Development

Edit the source files and the plugin watcher will automatically rebuild.

To reload after changes:
` + "```bash" + `
curl -X POST http://localhost:8080/api/v1/plugins/{plugin_id}/reload
` + "```" + `

## Configuration

Edit ` + "`config.schema.json`" + ` to add configuration options that will appear in the Admin UI.

## Documentation

- [Plugin SDK Documentation](https://github.com/TykTechnologies/midsommar/blob/main/pkg/plugin_sdk/README.md)
- [Plugin System Overview](https://github.com/TykTechnologies/midsommar/blob/main/docs/site/docs/plugins-overview.md)
`
