package marketplace

import (
	"time"
)

// PluginManifest represents the marketplace plugin manifest (manifest.yaml)
type PluginManifest struct {
	// Identity & Versioning
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description" json:"description"`

	// OCI Distribution
	OCI OCIInfo `yaml:"oci" json:"oci"`

	// Discovery & Classification
	Category string   `yaml:"category" json:"category"` // agents, connectors, tools, ui-extensions
	Keywords []string `yaml:"keywords" json:"keywords"`
	Maturity string   `yaml:"maturity" json:"maturity"` // alpha, beta, stable

	// Documentation & Support
	Links Links `yaml:"links" json:"links"`

	// Display Assets
	Icon        string   `yaml:"icon" json:"icon"`
	Screenshots []string `yaml:"screenshots" json:"screenshots"`

	// Provenance & Trust
	Maintainers []Maintainer `yaml:"maintainers" json:"maintainers"`
	Publisher   string       `yaml:"publisher" json:"publisher"` // tyk-official, tyk-verified, community
	License     string       `yaml:"license" json:"license"`

	// Capabilities (from plugin.manifest.json)
	Capabilities Capabilities `yaml:"capabilities" json:"capabilities"`

	// Requirements & Compatibility
	Requirements Requirements `yaml:"requirements" json:"requirements"`

	// Security & Permissions Preview
	Permissions Permissions `yaml:"permissions" json:"permissions"`

	// Installation Config Schema
	ConfigSchemaURL string `yaml:"config_schema_url" json:"config_schema_url"`

	// Verification
	Attestation Attestation `yaml:"attestation" json:"attestation"`

	// Metadata
	CreatedAt         time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt         time.Time `yaml:"updated_at" json:"updated_at"`
	Deprecated        bool      `yaml:"deprecated" json:"deprecated"`
	DeprecatedMessage string    `yaml:"deprecated_message" json:"deprecated_message"`
	Replacement       string    `yaml:"replacement" json:"replacement"` // Suggested replacement plugin ID
}

// OCIInfo contains OCI registry information
type OCIInfo struct {
	Registry   string   `yaml:"registry" json:"registry"`     // ghcr.io, nexus.example.com
	Repository string   `yaml:"repository" json:"repository"` // tyk-technologies/plugins/echo-agent
	Tag        string   `yaml:"tag" json:"tag"`               // 0.1.0
	Digest     string   `yaml:"digest" json:"digest"`         // sha256:abc123...
	Platform   []string `yaml:"platform" json:"platform"`     // ["linux/amd64", "linux/arm64", "darwin/amd64"]
}

// FullReference returns the complete OCI reference
func (o *OCIInfo) FullReference() string {
	if o.Digest != "" {
		return o.Registry + "/" + o.Repository + "@" + o.Digest
	}
	return o.Registry + "/" + o.Repository + ":" + o.Tag
}

// Links contains documentation and support links
type Links struct {
	Documentation string `yaml:"documentation" json:"documentation"`
	Repository    string `yaml:"repository" json:"repository"`
	Support       string `yaml:"support" json:"support"`
	Issues        string `yaml:"issues" json:"issues"`
	Homepage      string `yaml:"homepage" json:"homepage"`
}

// Maintainer represents a plugin maintainer
type Maintainer struct {
	Name         string `yaml:"name" json:"name"`
	Email        string `yaml:"email" json:"email"`
	Organization string `yaml:"organization" json:"organization"`
}

// Capabilities describes plugin hook capabilities
type Capabilities struct {
	Hooks       []string `yaml:"hooks" json:"hooks"`
	PrimaryHook string   `yaml:"primary_hook" json:"primary_hook"`
}

// Requirements describes compatibility requirements
type Requirements struct {
	MinStudioVersion string   `yaml:"min_studio_version" json:"min_studio_version"`
	APIVersions      []string `yaml:"api_versions" json:"api_versions"`
	Dependencies     []string `yaml:"dependencies" json:"dependencies"` // Other plugin IDs required
}

// Permissions describes required permissions
type Permissions struct {
	Services []string `yaml:"services" json:"services"` // ["llms.proxy", "tools.call"]
	KV       []string `yaml:"kv" json:"kv"`             // ["read", "write"]
	RPC      []string `yaml:"rpc" json:"rpc"`           // ["call"]
	UI       []string `yaml:"ui" json:"ui"`             // ["sidebar.register", "route.register"]
}

// Attestation describes verification information
type Attestation struct {
	Enabled         bool   `yaml:"enabled" json:"enabled"`
	SigstoreBundleURL string `yaml:"sigstore_bundle_url" json:"sigstore_bundle_url"`
}

// MarketplaceIndex represents the index.yaml file structure
type MarketplaceIndex struct {
	APIVersion string                      `yaml:"apiVersion" json:"apiVersion"`
	Generated  time.Time                   `yaml:"generated" json:"generated"`
	Plugins    map[string][]IndexedPlugin  `yaml:"plugins" json:"plugins"` // Key: plugin ID, Value: versions
}

// IndexedPlugin represents a plugin entry in the index (flattened for quick lookup)
type IndexedPlugin struct {
	ID               string    `yaml:"id" json:"id"`
	Name             string    `yaml:"name" json:"name"`
	Version          string    `yaml:"version" json:"version"`
	Description      string    `yaml:"description" json:"description"`
	Category         string    `yaml:"category" json:"category"`
	Maturity         string    `yaml:"maturity" json:"maturity"`
	Publisher        string    `yaml:"publisher" json:"publisher"`
	Icon             string    `yaml:"icon" json:"icon"`
	OCIRegistry      string    `yaml:"oci_registry" json:"oci_registry"`
	OCIRepository    string    `yaml:"oci_repository" json:"oci_repository"`
	OCITag           string    `yaml:"oci_tag" json:"oci_tag"`
	OCIDigest        string    `yaml:"oci_digest" json:"oci_digest"`
	OCIPlatform      []string  `yaml:"oci_platform" json:"oci_platform"`
	PrimaryHook      string    `yaml:"primary_hook" json:"primary_hook"`
	MinStudioVersion string    `yaml:"min_studio_version" json:"min_studio_version"`
	CreatedAt        time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt        time.Time `yaml:"updated_at" json:"updated_at"`
	Deprecated       bool      `yaml:"deprecated" json:"deprecated"`
	ManifestURL      string    `yaml:"manifest_url" json:"manifest_url"` // URL to full manifest.yaml

	// Permissions
	RequiredServices []string `yaml:"required_services" json:"required_services"`
	RequiredKV       []string `yaml:"required_kv" json:"required_kv"`
	RequiredRPC      []string `yaml:"required_rpc" json:"required_rpc"`
	RequiredUI       []string `yaml:"required_ui" json:"required_ui"`
}

// SearchFilters contains marketplace search and filter parameters
type SearchFilters struct {
	Query             string
	Category          string
	Publisher         string
	Maturity          string
	HookType          string
	IncludeDeprecated bool
	PageSize          int
	PageNumber        int
}

// InstallRequest represents a plugin installation request from marketplace
type InstallRequest struct {
	PluginID        string                 `json:"plugin_id" binding:"required"`
	Version         string                 `json:"version" binding:"required"`
	Config          map[string]interface{} `json:"config"`          // Initial configuration
	Name            string                 `json:"name"`            // Optional custom name
	Namespace       string                 `json:"namespace"`       // Optional namespace
	AutoUpdate      bool                   `json:"auto_update"`     // Enable auto-updates
	AcceptedScopes  []string               `json:"accepted_scopes"` // User-accepted permission scopes
}

// InstallResponse represents the response after plugin installation
type InstallResponse struct {
	Success         bool   `json:"success"`
	Message         string `json:"message"`
	PluginID        uint   `json:"plugin_id"`         // Database plugin ID
	MarketplaceID   string `json:"marketplace_id"`    // Marketplace plugin ID (e.g. com.tyk.echo-agent)
	Version         string `json:"version"`
	InstalledAt     time.Time `json:"installed_at"`
	RequiresApproval bool  `json:"requires_approval"` // If service scopes need admin approval
}

// UpdateCheckResponse represents available updates
type UpdateCheckResponse struct {
	UpdatesAvailable int                `json:"updates_available"`
	Plugins          []PluginUpdateInfo `json:"plugins"`
}

// PluginUpdateInfo describes an available update
type PluginUpdateInfo struct {
	PluginID         uint      `json:"plugin_id"`
	Name             string    `json:"name"`
	MarketplaceID    string    `json:"marketplace_id"`
	InstalledVersion string    `json:"installed_version"`
	AvailableVersion string    `json:"available_version"`
	Changelog        string    `json:"changelog"`
	BreakingChanges  bool      `json:"breaking_changes"`
	ReleaseDate      time.Time `json:"release_date"`
}

// SyncResult represents the result of a marketplace sync operation
type SyncResult struct {
	Success        bool      `json:"success"`
	PluginsAdded   int       `json:"plugins_added"`
	PluginsUpdated int       `json:"plugins_updated"`
	PluginsRemoved int       `json:"plugins_removed"`
	Errors         []string  `json:"errors"`
	LastSynced     time.Time `json:"last_synced"`
	Duration       string    `json:"duration"`
}
