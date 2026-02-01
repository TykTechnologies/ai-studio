package main

import "fmt"

// Capability represents a plugin capability with its constraints
type Capability struct {
	Name        string
	Description string
	StudioOnly  bool // Only available for studio plugins
	GatewayOnly bool // Only available for gateway plugins
}

// AllCapabilities defines all available plugin capabilities
var AllCapabilities = map[string]Capability{
	"pre_auth": {
		Name:        "pre_auth",
		Description: "Process requests before authentication",
		StudioOnly:  false,
		GatewayOnly: false,
	},
	"auth": {
		Name:        "auth",
		Description: "Custom authentication handler",
		StudioOnly:  false,
		GatewayOnly: false,
	},
	"post_auth": {
		Name:        "post_auth",
		Description: "Process requests after authentication",
		StudioOnly:  false,
		GatewayOnly: false,
	},
	"on_response": {
		Name:        "on_response",
		Description: "Modify response headers and body",
		StudioOnly:  false,
		GatewayOnly: false,
	},
	"studio_ui": {
		Name:        "studio_ui",
		Description: "Dashboard UI with WebComponents",
		StudioOnly:  true,
		GatewayOnly: false,
	},
	"object_hooks": {
		Name:        "object_hooks",
		Description: "Intercept CRUD operations on objects",
		StudioOnly:  true,
		GatewayOnly: false,
	},
	"data_collector": {
		Name:        "data_collector",
		Description: "Telemetry and analytics collection",
		StudioOnly:  false,
		GatewayOnly: false,
	},
	"agent": {
		Name:        "agent",
		Description: "Conversational AI agent with streaming",
		StudioOnly:  true,
		GatewayOnly: false,
	},
}

// validateCapability checks if a capability is valid for the given plugin type
func validateCapability(capName, pluginType string) error {
	cap, exists := AllCapabilities[capName]
	if !exists {
		validCaps := make([]string, 0, len(AllCapabilities))
		for name := range AllCapabilities {
			validCaps = append(validCaps, name)
		}
		return fmt.Errorf("unknown capability: %s (valid: %v)", capName, validCaps)
	}

	// Check studio-only capabilities
	if cap.StudioOnly && pluginType == "gateway" {
		return fmt.Errorf("capability %s is only available for studio plugins, not gateway", capName)
	}

	// Check gateway-only capabilities
	if cap.GatewayOnly && pluginType == "studio" {
		return fmt.Errorf("capability %s is only available for gateway plugins, not studio", capName)
	}

	// Agent type should only have agent capability
	if pluginType == "agent" && capName != "agent" {
		return fmt.Errorf("agent plugins only support the 'agent' capability")
	}

	// Data collector type should only have data_collector capability
	if pluginType == "data-collector" && capName != "data_collector" {
		return fmt.Errorf("data-collector plugins only support the 'data_collector' capability")
	}

	return nil
}
