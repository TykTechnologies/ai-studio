package main

import (
	_ "embed" // Used for //go:embed directives
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

//go:embed manifest.json
var manifestFile []byte

//go:embed config.schema.json
var configSchemaFile []byte

// LLMValidatorPlugin implements validation rules for LLM objects
type LLMValidatorPlugin struct {
	plugin_sdk.BasePlugin
	config *Config
}

type Config struct {
	RequireHTTPS       bool     `json:"require_https"`
	BlockedVendors     []string `json:"blocked_vendors"`
	MinPrivacyScore    int      `json:"min_privacy_score"`
	RequireDescription bool     `json:"require_description"`
}

// LLM represents the LLM object structure (subset of fields we care about)
type LLM struct {
	ID               uint     `json:"id"`
	Name             string   `json:"name"`
	APIEndpoint      string   `json:"api_endpoint"`
	Vendor           string   `json:"vendor"`
	PrivacyScore     int      `json:"privacy_score"`
	ShortDescription string   `json:"short_description"`
	Active           bool     `json:"active"`
	Metadata         map[string]interface{} `json:"metadata"`
}

func NewLLMValidatorPlugin() *LLMValidatorPlugin {
	return &LLMValidatorPlugin{
		config: &Config{
			RequireHTTPS:       true,
			BlockedVendors:     []string{},
			MinPrivacyScore:    0,
			RequireDescription: true,
		},
	}
}

// Init initializes the plugin with configuration
func (p *LLMValidatorPlugin) Init(ctx plugin_sdk.Context, config map[string]string) error {
	if configJSON, ok := config["config"]; ok {
		if err := json.Unmarshal([]byte(configJSON), p.config); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
	}

	// Debug: Print config to stderr for troubleshooting
	fmt.Fprintf(os.Stderr, "[llm-validator] Initialized with config: RequireHTTPS=%v, RequireDescription=%v, MinPrivacyScore=%d\n",
		p.config.RequireHTTPS, p.config.RequireDescription, p.config.MinPrivacyScore)

	// Log initialization (context logger not available during Init)
	_ = ctx // Use ctx to avoid unused variable warning

	return nil
}

// GetObjectHookRegistrations declares which hooks this plugin handles
func (p *LLMValidatorPlugin) GetObjectHookRegistrations() ([]*pb.ObjectHookRegistration, error) {
	return []*pb.ObjectHookRegistration{
		{
			ObjectType: "llm",
			HookTypes:  []string{"before_create", "before_update"},
			Priority:   10, // Run early in the chain
		},
	}, nil
}

// HandleObjectHook processes object hook invocations
func (p *LLMValidatorPlugin) HandleObjectHook(ctx plugin_sdk.Context, req *pb.ObjectHookRequest) (*pb.ObjectHookResponse, error) {
	// Only handle LLM objects
	if req.ObjectType != "llm" {
		return &pb.ObjectHookResponse{
			AllowOperation: true,
			Modified:       false,
		}, nil
	}

	// Parse LLM object
	var llm LLM
	if err := json.Unmarshal([]byte(req.ObjectJson), &llm); err != nil {
		return &pb.ObjectHookResponse{
			AllowOperation:  false,
			RejectionReason: fmt.Sprintf("Invalid LLM data: %v", err),
		}, nil
	}

	// Validation in progress...

	// Run validation checks
	if err := p.validateLLM(&llm); err != nil {
		return &pb.ObjectHookResponse{
			AllowOperation:  false,
			RejectionReason: err.Error(),
			Message:         fmt.Sprintf("LLM validation failed: %v", err),
		}, nil
	}

	// Add validation metadata
	metadata := map[string]string{
		"validated_by": "llm-validator",
		"validated_at": ctx.RequestID, // Use request ID as timestamp marker
		"validation_rules": fmt.Sprintf("https=%v,privacy>=%d",
			p.config.RequireHTTPS, p.config.MinPrivacyScore),
	}

	// Validation passed
	return &pb.ObjectHookResponse{
		AllowOperation: true,
		Modified:       false,
		PluginMetadata: metadata,
		Message:        fmt.Sprintf("LLM '%s' validated successfully", llm.Name),
	}, nil
}

// validateLLM performs validation checks
func (p *LLMValidatorPlugin) validateLLM(llm *LLM) error {
	// Debug: Print what we're validating
	fmt.Fprintf(os.Stderr, "[llm-validator] Validating LLM: Name=%s, APIEndpoint='%s', ShortDescription='%s'\n",
		llm.Name, llm.APIEndpoint, llm.ShortDescription)
	fmt.Fprintf(os.Stderr, "[llm-validator] Config: RequireHTTPS=%v, RequireDescription=%v\n",
		p.config.RequireHTTPS, p.config.RequireDescription)

	// Check HTTPS requirement
	if p.config.RequireHTTPS {
		if llm.APIEndpoint == "" {
			// Require endpoint to be set if HTTPS is required
			fmt.Fprintf(os.Stderr, "[llm-validator] HTTPS check FAILED: APIEndpoint is required but empty\n")
			return fmt.Errorf("API endpoint is required and must use HTTPS")
		} else if !strings.HasPrefix(strings.ToLower(llm.APIEndpoint), "https://") {
			fmt.Fprintf(os.Stderr, "[llm-validator] HTTPS check FAILED: endpoint='%s'\n", llm.APIEndpoint)
			return fmt.Errorf("API endpoint must use HTTPS (got: %s)", llm.APIEndpoint)
		} else {
			fmt.Fprintf(os.Stderr, "[llm-validator] HTTPS check passed: endpoint='%s'\n", llm.APIEndpoint)
		}
	}

	// Check blocked vendors
	for _, blocked := range p.config.BlockedVendors {
		if strings.EqualFold(llm.Vendor, blocked) {
			return fmt.Errorf("vendor '%s' is blocked by policy", llm.Vendor)
		}
	}

	// Check minimum privacy score
	if llm.PrivacyScore < p.config.MinPrivacyScore {
		return fmt.Errorf("privacy score %d is below minimum required %d",
			llm.PrivacyScore, p.config.MinPrivacyScore)
	}

	// Check description requirement
	if p.config.RequireDescription && strings.TrimSpace(llm.ShortDescription) == "" {
		return fmt.Errorf("short description is required")
	}

	return nil
}

// GetManifest implements plugin_sdk.ConfigProvider (required for installation)
func (p *LLMValidatorPlugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider (required for configuration UI)
func (p *LLMValidatorPlugin) GetConfigSchema() ([]byte, error) {
	return configSchemaFile, nil
}

func main() {
	plugin_sdk.Serve(NewLLMValidatorPlugin())
}
