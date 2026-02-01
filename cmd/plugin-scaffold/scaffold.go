package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// PluginConfig holds all configuration for generating a plugin
type PluginConfig struct {
	Name            string   // kebab-case: "my-rate-limiter"
	StructName      string   // PascalCase: "MyRateLimiter"
	DisplayName     string   // Title Case: "My Rate Limiter"
	Type            string   // studio, gateway, agent, data-collector
	Capabilities    []string // [post_auth, on_response, studio_ui]
	PrimaryHook     string   // First non-UI capability for manifest
	OutputDir       string   // Full path to output directory
	RelativeReplace string   // Relative path for go.mod replace directive

	// Computed flags for template conditionals
	HasUI             bool
	HasPostAuth       bool
	HasPreAuth        bool
	HasAuth           bool
	HasOnResponse     bool
	HasObjectHooks    bool
	HasDataCollector  bool
	HasSessionAware   bool // Needs OnSessionReady (for UI or agent)
}

// ValidTypes are the supported plugin types
var ValidTypes = []string{"studio", "gateway", "agent", "data-collector"}

// NewPluginConfig creates and validates a new plugin configuration
func NewPluginConfig(name, pluginType string, capabilities []string, customOutput string) (*PluginConfig, error) {
	// Validate type
	validType := false
	for _, t := range ValidTypes {
		if pluginType == t {
			validType = true
			break
		}
	}
	if !validType {
		return nil, fmt.Errorf("invalid plugin type: %s (valid: %s)", pluginType, strings.Join(ValidTypes, ", "))
	}

	// Set default capabilities based on type
	if len(capabilities) == 0 {
		switch pluginType {
		case "studio", "gateway":
			capabilities = []string{"post_auth"}
		case "agent":
			capabilities = []string{"agent"}
		case "data-collector":
			capabilities = []string{"data_collector"}
		}
	}

	// Validate capabilities for the plugin type
	for _, cap := range capabilities {
		if err := validateCapability(cap, pluginType); err != nil {
			return nil, err
		}
	}

	// Determine output directory
	outputDir := customOutput
	if outputDir == "" {
		switch pluginType {
		case "studio":
			outputDir = filepath.Join("examples", "plugins", "studio", name)
		case "gateway":
			outputDir = filepath.Join("examples", "plugins", "gateway", name)
		case "agent":
			outputDir = filepath.Join("examples", "plugins", "studio", name, "server")
		case "data-collector":
			outputDir = filepath.Join("examples", "plugins", "data-collectors", name)
		}
	}

	// Calculate relative path for go.mod replace
	relativeReplace := calculateRelativeReplace(outputDir)

	config := &PluginConfig{
		Name:            name,
		StructName:      toStructName(name),
		DisplayName:     toDisplayName(name),
		Type:            pluginType,
		Capabilities:    capabilities,
		OutputDir:       outputDir,
		RelativeReplace: relativeReplace,
	}

	// Set capability flags
	for _, cap := range capabilities {
		switch cap {
		case "studio_ui":
			config.HasUI = true
			config.HasSessionAware = true
		case "post_auth":
			config.HasPostAuth = true
		case "pre_auth":
			config.HasPreAuth = true
		case "auth":
			config.HasAuth = true
		case "on_response":
			config.HasOnResponse = true
		case "object_hooks":
			config.HasObjectHooks = true
		case "data_collector":
			config.HasDataCollector = true
		case "agent":
			config.HasSessionAware = true
		}
	}

	// Determine primary hook (first non-UI capability)
	for _, cap := range capabilities {
		if cap != "studio_ui" {
			config.PrimaryHook = cap
			break
		}
	}
	if config.PrimaryHook == "" && config.HasUI {
		config.PrimaryHook = "studio_ui"
	}

	return config, nil
}

// Scaffold generates all plugin files
func Scaffold(config *PluginConfig) error {
	fmt.Printf("Creating plugin: %s\n", config.Name)
	fmt.Printf("  Type: %s\n", config.Type)
	fmt.Printf("  Capabilities: %s\n", strings.Join(config.Capabilities, ", "))
	fmt.Printf("  Output: %s/\n", config.OutputDir)
	fmt.Println()

	// Check if directory already exists
	if _, err := os.Stat(config.OutputDir); !os.IsNotExist(err) {
		return fmt.Errorf("directory already exists: %s", config.OutputDir)
	}

	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate files based on plugin type
	var files []string
	var err error

	switch config.Type {
	case "studio":
		files, err = generateStudioPlugin(config)
	case "gateway":
		files, err = generateGatewayPlugin(config)
	case "agent":
		files, err = generateAgentPlugin(config)
	case "data-collector":
		files, err = generateDataCollectorPlugin(config)
	}

	if err != nil {
		// Clean up on error
		os.RemoveAll(config.OutputDir)
		return err
	}

	// Print generated files
	fmt.Println("Generated files:")
	for _, f := range files {
		fmt.Printf("  \u2713 %s\n", f)
	}

	// Print next steps
	printNextSteps(config)

	return nil
}

func generateStudioPlugin(config *PluginConfig) ([]string, error) {
	var files []string

	// main.go
	if err := writeTemplate(config, "studio/main.go.tmpl", "main.go"); err != nil {
		return nil, err
	}
	files = append(files, "main.go")

	// go.mod
	if err := writeTemplate(config, "studio/go.mod.tmpl", "go.mod"); err != nil {
		return nil, err
	}
	files = append(files, "go.mod")

	// manifest.json
	if err := writeTemplate(config, "studio/manifest.json.tmpl", "manifest.json"); err != nil {
		return nil, err
	}
	files = append(files, "manifest.json")

	// config.schema.json
	if err := writeTemplate(config, "studio/config.schema.json.tmpl", "config.schema.json"); err != nil {
		return nil, err
	}
	files = append(files, "config.schema.json")

	// README.md
	if err := writeTemplate(config, "common/README.md.tmpl", "README.md"); err != nil {
		return nil, err
	}
	files = append(files, "README.md")

	// UI assets if studio_ui capability
	if config.HasUI {
		uiFiles, err := generateUIAssets(config)
		if err != nil {
			return nil, err
		}
		files = append(files, uiFiles...)
	}

	return files, nil
}

func generateGatewayPlugin(config *PluginConfig) ([]string, error) {
	var files []string

	// main.go
	if err := writeTemplate(config, "gateway/main.go.tmpl", "main.go"); err != nil {
		return nil, err
	}
	files = append(files, "main.go")

	// go.mod
	if err := writeTemplate(config, "gateway/go.mod.tmpl", "go.mod"); err != nil {
		return nil, err
	}
	files = append(files, "go.mod")

	// manifest.json
	if err := writeTemplate(config, "gateway/manifest.json.tmpl", "manifest.json"); err != nil {
		return nil, err
	}
	files = append(files, "manifest.json")

	// README.md
	if err := writeTemplate(config, "common/README.md.tmpl", "README.md"); err != nil {
		return nil, err
	}
	files = append(files, "README.md")

	return files, nil
}

func generateAgentPlugin(config *PluginConfig) ([]string, error) {
	var files []string

	// main.go
	if err := writeTemplate(config, "agent/main.go.tmpl", "main.go"); err != nil {
		return nil, err
	}
	files = append(files, "main.go")

	// go.mod
	if err := writeTemplate(config, "agent/go.mod.tmpl", "go.mod"); err != nil {
		return nil, err
	}
	files = append(files, "go.mod")

	// plugin.manifest.json
	if err := writeTemplate(config, "agent/plugin.manifest.json.tmpl", "plugin.manifest.json"); err != nil {
		return nil, err
	}
	files = append(files, "plugin.manifest.json")

	// config.schema.json
	if err := writeTemplate(config, "agent/config.schema.json.tmpl", "config.schema.json"); err != nil {
		return nil, err
	}
	files = append(files, "config.schema.json")

	// README.md
	if err := writeTemplate(config, "common/README.md.tmpl", "README.md"); err != nil {
		return nil, err
	}
	files = append(files, "README.md")

	return files, nil
}

func generateDataCollectorPlugin(config *PluginConfig) ([]string, error) {
	var files []string

	// main.go
	if err := writeTemplate(config, "data-collector/main.go.tmpl", "main.go"); err != nil {
		return nil, err
	}
	files = append(files, "main.go")

	// go.mod
	if err := writeTemplate(config, "data-collector/go.mod.tmpl", "go.mod"); err != nil {
		return nil, err
	}
	files = append(files, "go.mod")

	// manifest.json
	if err := writeTemplate(config, "data-collector/manifest.json.tmpl", "manifest.json"); err != nil {
		return nil, err
	}
	files = append(files, "manifest.json")

	// README.md
	if err := writeTemplate(config, "common/README.md.tmpl", "README.md"); err != nil {
		return nil, err
	}
	files = append(files, "README.md")

	return files, nil
}

func generateUIAssets(config *PluginConfig) ([]string, error) {
	var files []string

	// Create ui/webc directory
	webcDir := filepath.Join(config.OutputDir, "ui", "webc")
	if err := os.MkdirAll(webcDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create ui/webc directory: %w", err)
	}

	// Create assets directory
	assetsDir := filepath.Join(config.OutputDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create assets directory: %w", err)
	}

	// dashboard.js
	if err := writeTemplate(config, "ui/dashboard.js.tmpl", filepath.Join("ui", "webc", "dashboard.js")); err != nil {
		return nil, err
	}
	files = append(files, "ui/webc/dashboard.js")

	// icon.svg
	if err := writeStaticFile(config.OutputDir, "assets/icon.svg", iconSVG); err != nil {
		return nil, err
	}
	files = append(files, "assets/icon.svg")

	return files, nil
}

func writeTemplate(config *PluginConfig, templateName, outputFile string) error {
	tmplContent, ok := templates[templateName]
	if !ok {
		return fmt.Errorf("template not found: %s", templateName)
	}

	tmpl, err := template.New(templateName).Parse(tmplContent)
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", templateName, err)
	}

	outputPath := filepath.Join(config.OutputDir, outputFile)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", outputFile, err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", outputFile, err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, config); err != nil {
		return fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return nil
}

func writeStaticFile(outputDir, relativePath, content string) error {
	outputPath := filepath.Join(outputDir, relativePath)
	return os.WriteFile(outputPath, []byte(content), 0644)
}

func printNextSteps(config *PluginConfig) {
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println()
	fmt.Printf("1. Build the plugin:\n")
	fmt.Printf("   cd %s && go build -o %s\n", config.OutputDir, config.Name)
	fmt.Println()
	fmt.Println("2. Start the dev environment:")
	fmt.Println("   make dev-full")
	fmt.Println()
	fmt.Println("3. Register in Admin UI (http://localhost:3000):")
	fmt.Println("   Admin > Plugins > Register Plugin")
	fmt.Printf("   Command: file:///app/%s/%s\n", config.OutputDir, config.Name)
	fmt.Println()
	fmt.Println("4. After changes, reload:")
	fmt.Println("   curl -X POST http://localhost:8080/api/v1/plugins/{id}/reload")
	fmt.Println()
	fmt.Println("The plugin watcher in 'make dev-full' auto-rebuilds on file changes.")
}

// Helper functions

func toStructName(kebab string) string {
	// Convert kebab-case to PascalCase
	parts := strings.Split(kebab, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

func toDisplayName(kebab string) string {
	// Convert kebab-case to Title Case
	parts := strings.Split(kebab, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func calculateRelativeReplace(outputDir string) string {
	// Calculate how many levels up to reach the project root
	depth := len(strings.Split(outputDir, string(os.PathSeparator)))
	parts := make([]string, depth)
	for i := range parts {
		parts[i] = ".."
	}
	return strings.Join(parts, "/")
}
