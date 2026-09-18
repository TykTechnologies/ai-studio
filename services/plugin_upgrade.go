package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/marketplace"
	"github.com/rs/zerolog/log"
	"github.com/xeipuuv/gojsonschema"
	"gorm.io/gorm"
)

// Upgrade errors the API maps to distinct responses.
var (
	// ErrUpgradeNotFromMarketplace: the plugin has no marketplace entry to upgrade from.
	ErrUpgradeNotFromMarketplace = errors.New("plugin is not linked to a marketplace entry")
	// ErrUpgradeVersionNotFound: the requested version is not published for this plugin.
	ErrUpgradeVersionNotFound = errors.New("version not found in the marketplace for this plugin")
	// ErrUpgradeSameVersion: the plugin already runs the requested artifact.
	ErrUpgradeSameVersion = errors.New("plugin is already on this version")
	// ErrUpgradeDowngradeNotAllowed: an older version was requested without allow_downgrade.
	ErrUpgradeDowngradeNotAllowed = errors.New("target version is older than the installed version")
	// ErrUpgradeManifestMismatch: the artifact is a different plugin from the one installed.
	ErrUpgradeManifestMismatch = errors.New("target artifact is a different plugin")
	// ErrUpgradeEnterpriseOnly: the target version needs the Enterprise edition.
	ErrUpgradeEnterpriseOnly = errors.New("target version requires the Enterprise edition")
	// ErrUpgradePluginNotFound: there is no such installed plugin.
	ErrUpgradePluginNotFound = errors.New("plugin not found")
	// ErrUpgradeTargetUnavailable: the target artifact could not be pulled,
	// verified or started. Nothing was changed.
	ErrUpgradeTargetUnavailable = errors.New("the target version could not be loaded")
)

// UpgradeScopesNotApprovedError is returned when the target version asks for
// scopes the admin has not approved in this request.
type UpgradeScopesNotApprovedError struct {
	Missing []string
}

func (e *UpgradeScopesNotApprovedError) Error() string {
	return fmt.Sprintf("the new version requests scopes that were not approved: %s", strings.Join(e.Missing, ", "))
}

// UpgradeRolledBackError is returned when the new version failed to start and
// the plugin was put back on the version it had.
type UpgradeRolledBackError struct {
	Cause       error
	RollbackErr error // non-nil when restoring the previous version also failed
}

func (e *UpgradeRolledBackError) Error() string {
	if e.RollbackErr != nil {
		return fmt.Sprintf("new version failed to start (%v) and restoring the previous version failed: %v", e.Cause, e.RollbackErr)
	}
	return fmt.Sprintf("new version failed to start, the previous version was restored: %v", e.Cause)
}

func (e *UpgradeRolledBackError) Unwrap() error { return e.Cause }

// pluginUpgradeProber starts a command just far enough to read its config
// schema and manifest, without touching any plugin row.
type pluginUpgradeProber interface {
	LoadPluginMetadata(ctx context.Context, command string) (*PluginMetadata, error)
}

// pluginUpgradeRuntime is the part of the Studio plugin manager an upgrade needs.
type pluginUpgradeRuntime interface {
	IsPluginLoaded(pluginID uint) bool
	UnloadPlugin(pluginID uint) error
	LoadPlugin(pluginID uint) error
	GetPluginManifest(pluginID uint) (string, error)
}

// pluginUIRegistrar re-registers a studio_ui plugin's routes and slots.
type pluginUIRegistrar interface {
	RegisterPluginUI(plugin *models.Plugin, manifest *models.PluginManifest) error
}

type managerUpgradeRuntime struct {
	manager *AIStudioPluginManager
}

func (r managerUpgradeRuntime) IsPluginLoaded(pluginID uint) bool {
	return r.manager.IsPluginLoaded(pluginID)
}
func (r managerUpgradeRuntime) UnloadPlugin(pluginID uint) error {
	return r.manager.UnloadPlugin(pluginID)
}
func (r managerUpgradeRuntime) LoadPlugin(pluginID uint) error {
	_, err := r.manager.LoadPlugin(pluginID)
	return err
}
func (r managerUpgradeRuntime) GetPluginManifest(pluginID uint) (string, error) {
	return r.manager.GetPluginManifest(pluginID)
}

// PluginUpgradeService moves an installed marketplace plugin to another
// published version in place. The plugin row - and with it the configuration
// and everything keyed on the plugin ID (KV data, agent configs, app and group
// resource bindings, LLM associations, schedules, RBAC grants) - is kept; only
// the artifact and what the artifact declares about itself change.
type PluginUpgradeService struct {
	db          *gorm.DB
	plugins     *PluginService
	marketplace *MarketplaceService
	prober      pluginUpgradeProber
	runtime     pluginUpgradeRuntime
	uiRegistrar pluginUIRegistrar

	// Upgrades are rare and swap a running process: one at a time on this node.
	mu sync.Mutex
}

// NewPluginUpgradeService wires the upgrade service to the running Studio services.
func NewPluginUpgradeService(db *gorm.DB, plugins *PluginService, marketplaceService *MarketplaceService, manager *AIStudioPluginManager, manifests *PluginManifestService) *PluginUpgradeService {
	s := &PluginUpgradeService{db: db, plugins: plugins, marketplace: marketplaceService}
	if manager != nil {
		s.prober = NewPluginMetadataLoader(db, manager)
		s.runtime = managerUpgradeRuntime{manager: manager}
	}
	if manifests != nil {
		s.uiRegistrar = manifests
	}
	return s
}

// PluginUpgradeRequest selects the version to move to.
type PluginUpgradeRequest struct {
	Version        string   `json:"version"`         // empty = latest upgrade candidate
	ApprovedScopes []string `json:"approved_scopes"` // must cover every scope the target adds
	AllowDowngrade bool     `json:"allow_downgrade"` // required to move to an older version
}

// UpgradeDiff describes how a list (scopes, hooks) changes between versions.
type UpgradeDiff struct {
	Current []string `json:"current"`
	Target  []string `json:"target"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

// UpgradeVersionOption is one entry of the version picker.
type UpgradeVersionOption struct {
	Version        string    `json:"version"`
	Deprecated     bool      `json:"deprecated"`
	EnterpriseOnly bool      `json:"enterprise_only"`
	Installed      bool      `json:"installed"`
	ReleasedAt     time.Time `json:"released_at"`
}

// PluginUpgradePreview is what an admin reviews before confirming an upgrade.
type PluginUpgradePreview struct {
	PluginID          uint                   `json:"plugin_id"`
	PluginName        string                 `json:"plugin_name"`
	MarketplaceID     string                 `json:"marketplace_id"`
	InstalledVersion  string                 `json:"installed_version"`
	TargetVersion     string                 `json:"target_version"`
	LatestVersion     string                 `json:"latest_version"`
	IsDowngrade       bool                   `json:"is_downgrade"`
	SameVersion       bool                   `json:"same_version"` // already on the target; nothing to apply
	TargetCommand     string                 `json:"target_command"`
	Scopes            UpgradeDiff            `json:"scopes"`
	Hooks             UpgradeDiff            `json:"hooks"`
	ConfigSchema      map[string]interface{} `json:"config_schema,omitempty"`
	ConfigIssues      []string               `json:"config_issues"`
	Warnings          []string               `json:"warnings"`
	Deprecated        bool                   `json:"deprecated"`
	DeprecatedMessage string                 `json:"deprecated_message,omitempty"`
	MinStudioVersion  string                 `json:"min_studio_version,omitempty"`
	AffectsEdges      bool                   `json:"affects_edges"`
	Changelog         string                 `json:"changelog"`
	Versions          []UpgradeVersionOption `json:"versions"`
}

// PluginUpgradeResult reports a completed upgrade.
type PluginUpgradeResult struct {
	Plugin       *models.Plugin `json:"-"`
	FromVersion  string         `json:"from_version"`
	ToVersion    string         `json:"to_version"`
	Reloaded     bool           `json:"reloaded"`
	AffectsEdges bool           `json:"affects_edges"`
	Warnings     []string       `json:"warnings"`
}

// upgradePlan is a vetted upgrade: everything resolved and probed, nothing written.
type upgradePlan struct {
	plugin        *models.Plugin
	tracking      *models.InstalledPluginVersion
	target        *models.MarketplacePlugin
	versions      []*models.MarketplacePlugin
	command       string
	metadata      *PluginMetadata
	manifestMap   map[string]interface{}
	scopes        UpgradeDiff
	hooks         UpgradeDiff
	primaryHook   string
	isDowngrade   bool
	warnings      []string
	latestVersion string
}

// Preview resolves and probes the target version and reports what would change.
// It writes nothing, but it does pull and start the new artifact.
func (s *PluginUpgradeService) Preview(ctx context.Context, pluginID uint, version string) (*PluginUpgradePreview, error) {
	plan, err := s.plan(ctx, pluginID, version)
	if errors.Is(err, ErrUpgradeSameVersion) && plan != nil {
		// Nothing to move to - already on the target. Still answer, so the
		// version picker can offer the other published versions.
		preview := &PluginUpgradePreview{
			PluginID:         plan.plugin.ID,
			PluginName:       plan.plugin.Name,
			MarketplaceID:    plan.tracking.MarketplacePluginID,
			InstalledVersion: plan.tracking.InstalledVersion,
			TargetVersion:    plan.target.Version,
			LatestVersion:    plan.latestVersion,
			SameVersion:      true,
			TargetCommand:    plan.command,
			Scopes:           diffLists(nil, nil),
			Hooks:            diffLists(nil, nil),
			ConfigIssues:     []string{},
			Warnings:         []string{},
			Versions:         upgradeVersionOptions(plan),
		}
		preview.Changelog = s.fetchChangelog(ctx, plan.target)
		return preview, nil
	}
	if err != nil {
		return nil, err
	}

	preview := &PluginUpgradePreview{
		PluginID:          plan.plugin.ID,
		PluginName:        plan.plugin.Name,
		MarketplaceID:     plan.tracking.MarketplacePluginID,
		InstalledVersion:  plan.tracking.InstalledVersion,
		TargetVersion:     plan.target.Version,
		LatestVersion:     plan.latestVersion,
		IsDowngrade:       plan.isDowngrade,
		TargetCommand:     plan.command,
		Scopes:            plan.scopes,
		Hooks:             plan.hooks,
		ConfigIssues:      []string{},
		Warnings:          plan.warnings,
		Deprecated:        plan.target.Deprecated,
		DeprecatedMessage: plan.target.DeprecatedMessage,
		MinStudioVersion:  plan.target.MinStudioVersion,
		AffectsEdges:      pluginAffectsEdges(plan.hooks.Target),
		Versions:          upgradeVersionOptions(plan),
	}

	if plan.metadata.ConfigSchema != "" {
		var schema map[string]interface{}
		if err := json.Unmarshal([]byte(plan.metadata.ConfigSchema), &schema); err == nil {
			preview.ConfigSchema = schema
			preview.ConfigIssues = validateConfigAgainstSchema(schema, plan.plugin.Config)
		}
	}

	preview.Changelog = s.fetchChangelog(ctx, plan.target)
	return preview, nil
}

// upgradeVersionOptions lists the published versions for the picker, newest first.
func upgradeVersionOptions(plan *upgradePlan) []UpgradeVersionOption {
	options := make([]UpgradeVersionOption, 0, len(plan.versions))
	seen := make(map[string]bool, len(plan.versions))
	for _, v := range plan.versions {
		if seen[v.Version] || v.OCIReference() == "" {
			continue
		}
		seen[v.Version] = true
		options = append(options, UpgradeVersionOption{
			Version:        v.Version,
			Deprecated:     v.Deprecated,
			EnterpriseOnly: v.EnterpriseOnly,
			Installed:      v.OCIReference() == plan.plugin.Command,
			ReleasedAt:     v.PluginCreatedAt,
		})
	}
	return options
}

// Upgrade moves the plugin to the requested version. The new artifact has to
// start with the existing configuration; if it does not, the previous version
// is restored and an *UpgradeRolledBackError is returned.
func (s *PluginUpgradeService) Upgrade(ctx context.Context, pluginID uint, req *PluginUpgradeRequest) (*PluginUpgradeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Planned again here, never taken from a client-held preview.
	plan, err := s.plan(ctx, pluginID, req.Version)
	if err != nil {
		return nil, err
	}
	if plan.isDowngrade && !req.AllowDowngrade {
		return nil, ErrUpgradeDowngradeNotAllowed
	}

	// Fail closed on permissions: every scope the target adds must be approved
	// in this request. Scopes the target no longer declares are dropped.
	if missing := subtract(plan.scopes.Added, req.ApprovedScopes); len(missing) > 0 {
		return nil, &UpgradeScopesNotApprovedError{Missing: missing}
	}

	plugin := plan.plugin
	previous := *plugin
	oldCommand := plugin.Command

	plugin.Command = plan.command
	plugin.OCIReference = plan.command
	// Edge gateways reload a plugin when its checksum or config changes. The
	// config is deliberately unchanged, so the checksum is what tells an edge
	// the artifact moved.
	plugin.Checksum = upgradeChecksum(plan.target)
	if plan.manifestMap != nil {
		plugin.Manifest = plan.manifestMap
	}
	plugin.ServiceScopes = plan.scopes.Target
	if len(plan.scopes.Target) > 0 {
		// Every target scope is either carried over from an approved set or
		// was approved above.
		plugin.ServiceAccessAuthorized = true
	}
	plugin.HookTypes, plugin.HookType, plugin.HookTypesCustomized = upgradedHooks(&previous, plan.hooks.Target, plan.primaryHook)

	if err := s.saveUpgradeColumns(plugin); err != nil {
		return nil, fmt.Errorf("failed to save upgraded plugin: %w", err)
	}

	reloaded, err := s.swapRunningPlugin(plugin)
	if err != nil {
		rollbackErr := s.rollback(plugin, &previous)
		return nil, &UpgradeRolledBackError{Cause: err, RollbackErr: rollbackErr}
	}

	s.plugins.syncPermissions(plugin, false)
	s.plugins.dropSchemaCacheIfUnshared(oldCommand, plugin.ID)
	if s.marketplace != nil {
		s.marketplace.OnPluginChanged(plugin.ID)
	}

	log.Info().
		Uint("plugin_id", plugin.ID).
		Str("plugin_name", plugin.Name).
		Str("from_version", plan.tracking.InstalledVersion).
		Str("to_version", plan.target.Version).
		Bool("reloaded", reloaded).
		Msg("Plugin upgraded from marketplace")

	return &PluginUpgradeResult{
		Plugin:       plugin,
		FromVersion:  plan.tracking.InstalledVersion,
		ToVersion:    plan.target.Version,
		Reloaded:     reloaded,
		AffectsEdges: pluginAffectsEdges(plugin.GetAllHookTypes()),
		Warnings:     plan.warnings,
	}, nil
}

// plan resolves the target version, probes its artifact and works out the diffs.
func (s *PluginUpgradeService) plan(ctx context.Context, pluginID uint, version string) (*upgradePlan, error) {
	plugin, err := s.plugins.GetPlugin(pluginID)
	if err != nil {
		var count int64
		if countErr := s.db.Model(&models.Plugin{}).Where("id = ?", pluginID).Count(&count).Error; countErr == nil && count == 0 {
			return nil, ErrUpgradePluginNotFound
		}
		return nil, err
	}
	if s.marketplace == nil {
		return nil, ErrUpgradeNotFromMarketplace
	}

	tracking, versions, err := s.marketplace.UpgradeTargets(plugin)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUpgradeNotFromMarketplace
		}
		return nil, err
	}

	latest := latestUpgradeCandidate(versions)
	var target *models.MarketplacePlugin
	if version == "" {
		target = latest
	} else {
		for _, v := range versions {
			if v.Version == version && v.OCIReference() != "" {
				target = v
				break
			}
		}
	}
	if target == nil {
		return nil, ErrUpgradeVersionNotFound
	}
	if target.EnterpriseOnly && !config.IsEnterprise() {
		return nil, ErrUpgradeEnterpriseOnly
	}

	command := target.OCIReference()
	if command == plugin.Command {
		plan := &upgradePlan{plugin: plugin, tracking: tracking, target: target, versions: versions, command: command}
		if latest != nil {
			plan.latestVersion = latest.Version
		}
		return plan, ErrUpgradeSameVersion
	}
	if err := s.plugins.validatePluginCommand(command); err != nil {
		return nil, fmt.Errorf("target command rejected: %w", err)
	}

	if s.prober == nil {
		return nil, fmt.Errorf("plugin manager not configured")
	}

	// Pulls the artifact (verifying its signature when that is required) and
	// starts it far enough to report its schema and manifest.
	metadata, err := s.prober.LoadPluginMetadata(ctx, command)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpgradeTargetUnavailable, err)
	}

	plan := &upgradePlan{
		plugin:   plugin,
		tracking: tracking,
		target:   target,
		versions: versions,
		command:  command,
		metadata: metadata,
		warnings: []string{},
	}
	if latest != nil {
		plan.latestVersion = latest.Version
	}
	plan.isDowngrade = tracking.InstalledVersion != "" && models.IsNewerVersion(tracking.InstalledVersion, target.Version)

	targetScopes := []string{}
	targetHooks := target.Hooks
	plan.primaryHook = target.PrimaryHook

	if manifest := metadata.Manifest; manifest != nil {
		expectedID, _ := plugin.Manifest["id"].(string)
		if expectedID == "" {
			expectedID = tracking.MarketplacePluginID
		}
		if manifest.ID != "" && expectedID != "" && manifest.ID != expectedID {
			return nil, fmt.Errorf("%w: installed %q, target artifact reports %q", ErrUpgradeManifestMismatch, expectedID, manifest.ID)
		}
		if manifest.Version != "" && manifest.Version != target.Version {
			plan.warnings = append(plan.warnings, fmt.Sprintf(
				"The artifact reports version %s but the marketplace lists it as %s.", manifest.Version, target.Version))
		}

		targetScopes = manifest.GetAllPermissionScopes()
		if manifest.Capabilities != nil && len(manifest.Capabilities.Hooks) > 0 {
			targetHooks = manifest.Capabilities.Hooks
			plan.primaryHook = manifest.Capabilities.PrimaryHook
		}

		manifestJSON, err := json.Marshal(manifest)
		if err == nil {
			err = json.Unmarshal(manifestJSON, &plan.manifestMap)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read the target manifest: %w", err)
		}
	}
	if len(targetHooks) == 0 {
		return nil, fmt.Errorf("the target version declares no hook types")
	}

	// Only scopes an admin actually approved count as current.
	currentScopes := []string{}
	if plugin.ServiceAccessAuthorized {
		currentScopes = plugin.ServiceScopes
	}
	plan.scopes = diffLists(currentScopes, targetScopes)
	plan.hooks = diffLists(plugin.GetAllHookTypes(), targetHooks)

	return plan, nil
}

// saveUpgradeColumns writes exactly the columns an upgrade owns. Config is not
// among them. Select+Updates goes through the JSON serializers and writes
// zero values (an emptied scope list, a cleared customised flag).
func (s *PluginUpgradeService) saveUpgradeColumns(plugin *models.Plugin) error {
	return s.db.Model(plugin).
		Select("Command", "OCIReference", "Checksum", "Manifest", "ServiceScopes", "ServiceAccessAuthorized", "HookType", "HookTypes", "HookTypesCustomized").
		Updates(plugin).Error
}

// swapRunningPlugin restarts the plugin on the new artifact when Studio runs
// it. Initialize runs with the existing configuration, which makes a
// successful load the health check. Plugins Studio does not run (gateway-only
// hooks, inactive plugins) were already started once by the probe.
func (s *PluginUpgradeService) swapRunningPlugin(plugin *models.Plugin) (bool, error) {
	if s.runtime == nil || !plugin.IsActive {
		return false, nil
	}
	wasLoaded := s.runtime.IsPluginLoaded(plugin.ID)
	if !wasLoaded && !pluginRunsInStudio(plugin.GetAllHookTypes()) {
		return false, nil
	}

	if wasLoaded {
		if err := s.runtime.UnloadPlugin(plugin.ID); err != nil {
			log.Warn().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to unload plugin before upgrade, continuing")
		}
	}
	if err := s.runtime.LoadPlugin(plugin.ID); err != nil {
		return false, err
	}

	if plugin.SupportsHookType(models.HookTypeStudioUI) && s.uiRegistrar != nil {
		manifestJSON, err := s.runtime.GetPluginManifest(plugin.ID)
		if err != nil {
			return false, fmt.Errorf("failed to fetch manifest from the new version: %w", err)
		}
		manifest := &models.PluginManifest{}
		if err := json.Unmarshal([]byte(manifestJSON), manifest); err != nil {
			return false, fmt.Errorf("failed to parse manifest from the new version: %w", err)
		}
		if err := s.uiRegistrar.RegisterPluginUI(plugin, manifest); err != nil {
			return false, fmt.Errorf("failed to register UI for the new version: %w", err)
		}
	}
	return true, nil
}

// rollback puts the previous artifact back and restarts it. The previous
// artifact is still in the digest-addressed OCI cache.
func (s *PluginUpgradeService) rollback(plugin *models.Plugin, previous *models.Plugin) error {
	plugin.Command = previous.Command
	plugin.OCIReference = previous.OCIReference
	plugin.Checksum = previous.Checksum
	plugin.Manifest = previous.Manifest
	plugin.ServiceScopes = previous.ServiceScopes
	plugin.ServiceAccessAuthorized = previous.ServiceAccessAuthorized
	plugin.HookType = previous.HookType
	plugin.HookTypes = previous.HookTypes
	plugin.HookTypesCustomized = previous.HookTypesCustomized

	if err := s.saveUpgradeColumns(plugin); err != nil {
		log.Error().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to restore plugin row after a failed upgrade")
		return err
	}

	if s.runtime != nil && plugin.IsActive {
		if s.runtime.IsPluginLoaded(plugin.ID) {
			if err := s.runtime.UnloadPlugin(plugin.ID); err != nil {
				log.Warn().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to unload plugin during rollback")
			}
		}
		if err := s.runtime.LoadPlugin(plugin.ID); err != nil {
			log.Error().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to restart the previous plugin version after a failed upgrade")
			return err
		}
		if plugin.SupportsHookType(models.HookTypeStudioUI) && s.uiRegistrar != nil {
			if manifestJSON, err := s.runtime.GetPluginManifest(plugin.ID); err == nil {
				manifest := &models.PluginManifest{}
				if json.Unmarshal([]byte(manifestJSON), manifest) == nil {
					if err := s.uiRegistrar.RegisterPluginUI(plugin, manifest); err != nil {
						log.Warn().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to re-register UI during rollback")
					}
				}
			}
		}
	}
	return nil
}

// fetchChangelog is best effort: an upgrade never depends on it.
func (s *PluginUpgradeService) fetchChangelog(ctx context.Context, target *models.MarketplacePlugin) string {
	if s.marketplace == nil || s.marketplace.fetcher == nil {
		return ""
	}
	changelogURL, err := marketplace.ChangelogURL(target.ManifestURL, target.SyncedFromURL)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	changelog, err := s.marketplace.fetcher.FetchChangelog(ctx, changelogURL)
	if err != nil {
		log.Debug().Err(err).Str("url", changelogURL).Msg("No changelog available for marketplace plugin version")
		return ""
	}
	return changelog
}

// dropSchemaCacheIfUnshared removes the cached config schema of a command no
// other plugin uses any more.
func (s *PluginService) dropSchemaCacheIfUnshared(command string, exceptPluginID uint) {
	if command == "" {
		return
	}
	var others int64
	if err := s.db.Model(&models.Plugin{}).Where("command = ? AND id != ?", command, exceptPluginID).Count(&others).Error; err != nil || others > 0 {
		return
	}
	if err := s.db.Where("command = ?", command).Delete(&models.PluginConfigSchema{}).Error; err != nil {
		log.Warn().Err(err).Str("command", command).Msg("Failed to clean up config schema of the previous plugin version")
	}
}

// upgradeChecksum identifies the artifact a plugin row points at.
func upgradeChecksum(target *models.MarketplacePlugin) string {
	if target.OCIDigest != "" {
		return target.OCIDigest
	}
	return "version:" + target.Version
}

// upgradedHooks applies the target's hooks. A hook set the admin customised is
// kept as far as the new version still supports it.
func upgradedHooks(previous *models.Plugin, targetHooks []string, targetPrimary string) ([]string, string, bool) {
	hooks := targetHooks
	customized := false
	if previous.HookTypesCustomized {
		if kept := intersect(previous.HookTypes, targetHooks); len(kept) > 0 {
			hooks = kept
			customized = true
		}
	}
	primary := previous.HookType
	if !contains(hooks, primary) {
		primary = targetPrimary
	}
	if !contains(hooks, primary) {
		primary = hooks[0]
	}
	return hooks, primary, customized
}

var gatewayHookTypes = []string{
	models.HookTypePreAuth,
	models.HookTypeAuth,
	models.HookTypePostAuth,
	models.HookTypeOnResponse,
	models.HookTypeDataCollection,
	models.HookTypeCustomEndpoint,
}

var studioHookTypes = []string{
	models.HookTypeStudioUI,
	models.HookTypePortalUI,
	models.HookTypeAgent,
	models.HookTypeObjectHooks,
	models.HookTypeResourceProvider,
}

// pluginAffectsEdges reports whether edge gateways run this plugin, in which
// case an upgrade has to be pushed to them.
func pluginAffectsEdges(hooks []string) bool {
	return len(intersect(hooks, gatewayHookTypes)) > 0
}

func pluginRunsInStudio(hooks []string) bool {
	return len(intersect(hooks, studioHookTypes)) > 0
}

// validateConfigAgainstSchema lists where the existing configuration does not
// fit the target version's schema. It is advisory: schemas are of uneven
// quality, and the plugin's own Initialize is the real test. Schemas without
// properties (the fallback schema) are not checked.
func validateConfigAgainstSchema(schema map[string]interface{}, pluginConfig map[string]interface{}) []string {
	issues := []string{}
	if props, _ := schema["properties"].(map[string]interface{}); len(props) == 0 {
		return issues
	}
	if pluginConfig == nil {
		pluginConfig = map[string]interface{}{}
	}
	result, err := gojsonschema.Validate(gojsonschema.NewGoLoader(schema), gojsonschema.NewGoLoader(pluginConfig))
	if err != nil {
		return issues
	}
	for _, e := range result.Errors() {
		issues = append(issues, e.String())
	}
	return issues
}

func diffLists(current, target []string) UpgradeDiff {
	diff := UpgradeDiff{
		Current: normalizeList(current),
		Target:  normalizeList(target),
	}
	diff.Added = subtract(diff.Target, diff.Current)
	diff.Removed = subtract(diff.Current, diff.Target)
	return diff
}

func normalizeList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// subtract returns the items of a that are not in b.
func subtract(a, b []string) []string {
	out := []string{}
	for _, v := range a {
		if !contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}

func intersect(a, b []string) []string {
	out := []string{}
	for _, v := range a {
		if contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}
