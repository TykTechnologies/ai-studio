// Package main provides a CLI tool for scaffolding new Tyk AI Studio plugins.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	// Define flags
	name := flag.String("name", "", "Plugin name in kebab-case (e.g., my-rate-limiter)")
	pluginType := flag.String("type", "", "Plugin type: studio, gateway, agent, data-collector")
	capabilities := flag.String("capabilities", "", "Comma-separated capabilities (e.g., post_auth,on_response,studio_ui)")
	outputDir := flag.String("output", "", "Custom output directory (default: auto-detect based on type)")
	showHelp := flag.Bool("help", false, "Show help message")

	flag.Parse()

	if *showHelp || (*name == "" && *pluginType == "") {
		printHelp()
		os.Exit(0)
	}

	// Validate required flags
	if *name == "" {
		fmt.Println("Error: -name is required")
		fmt.Println("Usage: plugin-scaffold -name=my-plugin -type=studio [-capabilities=post_auth,on_response]")
		os.Exit(1)
	}

	if *pluginType == "" {
		fmt.Println("Error: -type is required")
		fmt.Println("Usage: plugin-scaffold -name=my-plugin -type=studio [-capabilities=post_auth,on_response]")
		os.Exit(1)
	}

	// Validate plugin name format (kebab-case)
	if !isValidKebabCase(*name) {
		fmt.Printf("Error: Plugin name must be kebab-case (lowercase letters, numbers, and hyphens): %s\n", *name)
		os.Exit(1)
	}

	// Parse capabilities
	var caps []string
	if *capabilities != "" {
		caps = strings.Split(*capabilities, ",")
		for i, cap := range caps {
			caps[i] = strings.TrimSpace(cap)
		}
	}

	// Create plugin config
	config, err := NewPluginConfig(*name, *pluginType, caps, *outputDir)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Run scaffolding
	if err := Scaffold(config); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`Plugin Scaffolding System for Tyk AI Studio

Usage:
  plugin-scaffold -name=<name> -type=<type> [-capabilities=<cap1,cap2,...>]

Required Flags:
  -name          Plugin name in kebab-case (e.g., my-rate-limiter)
  -type          Plugin type: studio, gateway, agent, data-collector

Optional Flags:
  -capabilities  Comma-separated capabilities to include
  -output        Custom output directory (default: auto-detect)
  -help          Show this help message

Plugin Types:
  studio         AI Studio plugin (runs in control plane)
  gateway        Microgateway plugin (runs in data plane)
  agent          Conversational AI agent with streaming
  data-collector Telemetry and analytics collector

Available Capabilities:
  pre_auth       Process before authentication
  auth           Custom authentication handler
  post_auth      Process after authentication (default for studio/gateway)
  on_response    Modify response headers and body
  studio_ui      Dashboard UI with WebComponents (studio only)
  object_hooks   CRUD operation interception (studio only)
  data_collector Telemetry collection handlers

Examples:
  # Basic studio plugin with post_auth (default)
  plugin-scaffold -name=my-limiter -type=studio

  # Multi-capability caching plugin with UI
  plugin-scaffold -name=my-cache -type=studio -capabilities=studio_ui,post_auth,on_response

  # Gateway plugin with request/response hooks
  plugin-scaffold -name=my-filter -type=gateway -capabilities=post_auth,on_response

  # Conversational agent
  plugin-scaffold -name=my-assistant -type=agent

  # Data collector for telemetry
  plugin-scaffold -name=my-exporter -type=data-collector

Output Directories:
  studio         examples/plugins/studio/<name>/
  gateway        examples/plugins/gateway/<name>/
  agent          examples/plugins/studio/<name>/server/
  data-collector examples/plugins/data-collectors/<name>/

After scaffolding:
  1. cd <output-dir> && go build -o <name>
  2. make dev-full (starts plugin watcher)
  3. Register in Admin UI: file:///app/<output-dir>/<name>
  4. Reload after changes: curl -X POST localhost:8080/api/v1/plugins/{id}/reload`)
}

func isValidKebabCase(s string) bool {
	if s == "" {
		return false
	}
	// Must start with lowercase letter
	if s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	// Cannot end with hyphen
	if s[len(s)-1] == '-' {
		return false
	}
	// No double hyphens
	if strings.Contains(s, "--") {
		return false
	}
	return true
}
