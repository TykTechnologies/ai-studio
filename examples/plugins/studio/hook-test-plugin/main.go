package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

//go:embed ui manifest.json config.schema.json
var embeddedAssets embed.FS

//go:embed manifest.json
var manifestFile []byte

//go:embed config.schema.json
var configSchemaFile []byte

// HookTestPlugin implements all object hooks for comprehensive testing
type HookTestPlugin struct {
	plugin_sdk.BasePlugin
	config *Config
}

// Config defines behavior for each hook type
type Config struct {
	// Global settings
	EnableLogging bool `json:"enable_logging"` // Log all hook invocations

	// Per-hook-type configuration
	BeforeCreate HookConfig `json:"before_create"`
	AfterCreate  HookConfig `json:"after_create"`
	BeforeUpdate HookConfig `json:"before_update"`
	AfterUpdate  HookConfig `json:"after_update"`
	BeforeDelete HookConfig `json:"before_delete"`
	AfterDelete  HookConfig `json:"after_delete"`
}

// HookConfig defines behavior for a specific hook type
type HookConfig struct {
	Mode            string            `json:"mode"`              // "allow", "reject", "modify", "metadata"
	RejectionReason string            `json:"rejection_reason"`  // Message when rejecting
	ModifyField     string            `json:"modify_field"`      // Field to modify (e.g., "name")
	ModifyValue     string            `json:"modify_value"`      // New value for field
	MetadataKey     string            `json:"metadata_key"`      // Metadata key to add
	MetadataValue   string            `json:"metadata_value"`    // Metadata value to add
}

func NewHookTestPlugin() *HookTestPlugin {
	return &HookTestPlugin{
		config: &Config{
			EnableLogging: true,
			BeforeCreate:  HookConfig{Mode: "allow"},
			AfterCreate:   HookConfig{Mode: "allow"},
			BeforeUpdate:  HookConfig{Mode: "allow"},
			AfterUpdate:   HookConfig{Mode: "allow"},
			BeforeDelete:  HookConfig{Mode: "allow"},
			AfterDelete:   HookConfig{Mode: "allow"},
		},
	}
}

// Init initializes the plugin with configuration
func (p *HookTestPlugin) Init(ctx plugin_sdk.Context, config map[string]string) error {
	if configJSON, ok := config["config"]; ok {
		if err := json.Unmarshal([]byte(configJSON), p.config); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
	}

	if p.config.EnableLogging {
		fmt.Fprintf(os.Stderr, "[hook-test-plugin] Initialized at %s\n", time.Now().Format(time.RFC3339))
		fmt.Fprintf(os.Stderr, "[hook-test-plugin] Configuration:\n")
		fmt.Fprintf(os.Stderr, "  before_create: %s\n", p.config.BeforeCreate.Mode)
		fmt.Fprintf(os.Stderr, "  after_create: %s\n", p.config.AfterCreate.Mode)
		fmt.Fprintf(os.Stderr, "  before_update: %s\n", p.config.BeforeUpdate.Mode)
		fmt.Fprintf(os.Stderr, "  after_update: %s\n", p.config.AfterUpdate.Mode)
		fmt.Fprintf(os.Stderr, "  before_delete: %s\n", p.config.BeforeDelete.Mode)
		fmt.Fprintf(os.Stderr, "  after_delete: %s\n", p.config.AfterDelete.Mode)
	}

	return nil
}

// GetObjectHookRegistrations declares all hooks this plugin handles
func (p *HookTestPlugin) GetObjectHookRegistrations() ([]*pb.ObjectHookRegistration, error) {
	return []*pb.ObjectHookRegistration{
		{
			ObjectType: "llm",
			HookTypes:  []string{"before_create", "after_create", "before_update", "after_update", "before_delete", "after_delete"},
			Priority:   50, // Mid-priority
		},
		{
			ObjectType: "datasource",
			HookTypes:  []string{"before_create", "after_create", "before_update", "after_update", "before_delete", "after_delete"},
			Priority:   50,
		},
		{
			ObjectType: "tool",
			HookTypes:  []string{"before_create", "after_create", "before_update", "after_update", "before_delete", "after_delete"},
			Priority:   50,
		},
		{
			ObjectType: "user",
			HookTypes:  []string{"before_create", "after_create", "before_update", "after_update", "before_delete", "after_delete"},
			Priority:   50,
		},
	}, nil
}

// HandleObjectHook processes object hook invocations
func (p *HookTestPlugin) HandleObjectHook(ctx plugin_sdk.Context, req *pb.ObjectHookRequest) (*pb.ObjectHookResponse, error) {
	// Log invocation
	if p.config.EnableLogging {
		fmt.Fprintf(os.Stderr, "[hook-test-plugin] Hook invoked: object_type=%s, hook_type=%s, operation_id=%s\n",
			req.ObjectType, req.HookType, req.OperationId)
	}

	// Get configuration for this hook type
	hookConfig := p.getHookConfig(req.HookType)

	// Parse object for potential modification
	var objData map[string]interface{}
	if err := json.Unmarshal([]byte(req.ObjectJson), &objData); err != nil {
		return &pb.ObjectHookResponse{
			AllowOperation:  false,
			RejectionReason: fmt.Sprintf("Failed to parse object: %v", err),
		}, nil
	}

	// Execute based on mode
	switch hookConfig.Mode {
	case "reject":
		if p.config.EnableLogging {
			fmt.Fprintf(os.Stderr, "[hook-test-plugin] Rejecting operation: %s\n", hookConfig.RejectionReason)
		}
		return &pb.ObjectHookResponse{
			AllowOperation:  false,
			RejectionReason: hookConfig.RejectionReason,
			Message:         fmt.Sprintf("Operation blocked by hook-test-plugin: %s", hookConfig.RejectionReason),
		}, nil

	case "modify":
		// Modify the specified field
		if hookConfig.ModifyField != "" {
			oldValue := objData[hookConfig.ModifyField]
			objData[hookConfig.ModifyField] = hookConfig.ModifyValue

			if p.config.EnableLogging {
				fmt.Fprintf(os.Stderr, "[hook-test-plugin] Modified field '%s': '%v' -> '%s'\n",
					hookConfig.ModifyField, oldValue, hookConfig.ModifyValue)
			}

			// Marshal modified object
			modifiedJSON, err := json.Marshal(objData)
			if err != nil {
				return &pb.ObjectHookResponse{
					AllowOperation:  false,
					RejectionReason: fmt.Sprintf("Failed to marshal modified object: %v", err),
				}, nil
			}

			return &pb.ObjectHookResponse{
				AllowOperation:     true,
				Modified:           true,
				ModifiedObjectJson: string(modifiedJSON),
				Message:            fmt.Sprintf("Modified %s.%s by hook-test-plugin", req.ObjectType, hookConfig.ModifyField),
			}, nil
		}
		// If no field specified, just allow
		return &pb.ObjectHookResponse{
			AllowOperation: true,
			Modified:       false,
		}, nil

	case "metadata":
		// Add metadata
		metadata := make(map[string]string)
		if hookConfig.MetadataKey != "" {
			metadata[hookConfig.MetadataKey] = hookConfig.MetadataValue

			if p.config.EnableLogging {
				fmt.Fprintf(os.Stderr, "[hook-test-plugin] Adding metadata: %s=%s\n",
					hookConfig.MetadataKey, hookConfig.MetadataValue)
			}
		}

		return &pb.ObjectHookResponse{
			AllowOperation: true,
			Modified:       false,
			PluginMetadata: metadata,
			Message:        fmt.Sprintf("Metadata added by hook-test-plugin"),
		}, nil

	case "allow":
		fallthrough
	default:
		// Just allow the operation
		if p.config.EnableLogging {
			fmt.Fprintf(os.Stderr, "[hook-test-plugin] Allowing operation\n")
		}
		return &pb.ObjectHookResponse{
			AllowOperation: true,
			Modified:       false,
		}, nil
	}
}

// getHookConfig returns the configuration for a specific hook type
func (p *HookTestPlugin) getHookConfig(hookType string) HookConfig {
	switch hookType {
	case "before_create":
		return p.config.BeforeCreate
	case "after_create":
		return p.config.AfterCreate
	case "before_update":
		return p.config.BeforeUpdate
	case "after_update":
		return p.config.AfterUpdate
	case "before_delete":
		return p.config.BeforeDelete
	case "after_delete":
		return p.config.AfterDelete
	default:
		return HookConfig{Mode: "allow"}
	}
}

// GetManifest implements plugin_sdk.ManifestProvider
func (p *HookTestPlugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *HookTestPlugin) GetConfigSchema() ([]byte, error) {
	return configSchemaFile, nil
}

// GetAsset implements plugin_sdk.UIProvider
func (p *HookTestPlugin) GetAsset(assetPath string) ([]byte, string, error) {
	if len(assetPath) > 0 && assetPath[0] == '/' {
		assetPath = assetPath[1:]
	}

	content, err := embeddedAssets.ReadFile(assetPath)
	if err != nil {
		return nil, "", fmt.Errorf("asset not found: %s", assetPath)
	}

	mimeType := "application/octet-stream"
	if strings.HasSuffix(assetPath, ".html") {
		mimeType = "text/html"
	} else if strings.HasSuffix(assetPath, ".css") {
		mimeType = "text/css"
	} else if strings.HasSuffix(assetPath, ".js") {
		mimeType = "application/javascript"
	}

	return content, mimeType, nil
}

// ListAssets implements plugin_sdk.UIProvider
func (p *HookTestPlugin) ListAssets(pathPrefix string) ([]*pb.AssetInfo, error) {
	return []*pb.AssetInfo{
		{Path: "ui/index.html", MimeType: "text/html"},
		{Path: "ui/styles.css", MimeType: "text/css"},
		{Path: "ui/app.js", MimeType: "application/javascript"},
	}, nil
}

// HandleRPC implements plugin_sdk.UIProvider for handling test execution
func (p *HookTestPlugin) HandleRPC(method string, payload []byte) ([]byte, error) {
	fmt.Fprintf(os.Stderr, "[hook-test-plugin] RPC call: method=%s\n", method)

	switch method {
	case "run_single_test":
		return p.handleRunSingleTest(payload)
	default:
		return json.Marshal(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Unknown method: %s", method),
		})
	}
}

// handleRunSingleTest handles the run_single_test RPC call
func (p *HookTestPlugin) handleRunSingleTest(payload []byte) ([]byte, error) {
	startTime := time.Now()

	// Parse request
	var testReq struct {
		ObjectType string `json:"object_type"`
		HookType   string `json:"hook_type"`
	}
	if err := json.Unmarshal(payload, &testReq); err != nil {
		return json.Marshal(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Invalid request: %v", err),
		})
	}

	// Validate inputs
	if testReq.ObjectType == "" || testReq.HookType == "" {
		return json.Marshal(map[string]interface{}{
			"success": false,
			"error":   "object_type and hook_type are required",
		})
	}

	// Run the test
	result := p.runSingleTest(testReq.ObjectType, testReq.HookType)
	result["duration"] = time.Since(startTime).Seconds()

	return json.Marshal(result)
}

// runSingleTest performs a test for a specific object/hook combination
func (p *HookTestPlugin) runSingleTest(objectType, hookType string) map[string]interface{} {
	// Create a test object based on type
	testObject := p.createTestObject(objectType)

	// Marshal to JSON
	objectJSON, err := json.Marshal(testObject)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to marshal test object: %v", err),
		}
	}

	// Create hook request
	hookReq := &pb.ObjectHookRequest{
		HookType:    hookType,
		ObjectType:  objectType,
		OperationId: fmt.Sprintf("test-%s-%s-%d", objectType, hookType, time.Now().Unix()),
		UserId:      1,
		PluginId:    1,
		ObjectJson:  string(objectJSON),
		Metadata:    map[string]string{"test": "true"},
		Timestamp:   time.Now().Unix(),
	}

	// Execute the hook (with minimal context for testing)
	ctx := plugin_sdk.Context{
		RequestID: fmt.Sprintf("test-%d", time.Now().Unix()),
		Runtime:   "test",
	}
	hookResp, err := p.HandleObjectHook(ctx, hookReq)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Hook execution error: %v", err),
		}
	}

	// Verify the hook was called and responded appropriately
	message := fmt.Sprintf("Hook %s.%s executed successfully", objectType, hookType)
	if hookResp.Message != "" {
		message = hookResp.Message
	}

	return map[string]interface{}{
		"success": true,
		"message": message,
		"details": map[string]interface{}{
			"allowed":  hookResp.AllowOperation,
			"modified": hookResp.Modified,
			"metadata": hookResp.PluginMetadata,
		},
	}
}

// createTestObject creates a test object for the given type
func (p *HookTestPlugin) createTestObject(objectType string) map[string]interface{} {
	switch objectType {
	case "llm":
		return map[string]interface{}{
			"id":                0,
			"name":              "test-llm",
			"api_endpoint":      "https://api.test.com",
			"vendor":            "test-vendor",
			"privacy_score":     50,
			"short_description": "Test LLM for hook testing",
			"active":            true,
		}
	case "datasource":
		return map[string]interface{}{
			"id":          0,
			"name":        "test-datasource",
			"description": "Test datasource for hook testing",
			"type":        "rest_api",
			"active":      true,
		}
	case "tool":
		return map[string]interface{}{
			"id":          0,
			"name":        "test-tool",
			"description": "Test tool for hook testing",
			"type":        "api",
			"active":      true,
		}
	case "user":
		return map[string]interface{}{
			"id":       0,
			"username": "test-user",
			"email":    "test@example.com",
			"role":     "user",
			"active":   true,
		}
	default:
		return map[string]interface{}{
			"id":   0,
			"name": "test-object",
		}
	}
}

func main() {
	plugin_sdk.Serve(NewHookTestPlugin())
}
