package services

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/mcpannotations"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
)

// The app guardrail capability (features/GuardrailExceptions.md): what a full
// administrator attests an app can do, which bounds how far a guardrail
// exception may soften a verdict for it. Studio suggests a value from what it
// can see of the app (its tools, MCP servers and datasources); the
// administrator attests, since the gateway cannot see everything the app can
// do (a CI job's own GitHub token, say).

// ErrInvalidGuardrailCapability is a capability value the service refuses.
var ErrInvalidGuardrailCapability = errors.New("invalid guardrail capability")

// CapabilitySuggestion is Studio's suggested capability for an app and why.
type CapabilitySuggestion struct {
	Suggestion guardrails.Capability `json:"suggestion"`
	// Reasons explain the suggestion. For acting they name each tool
	// operation or MCP primitive that can act; otherwise what was seen.
	Reasons []string `json:"reasons"`
}

// SuggestAppCapability suggests a capability from the app's tools, MCP
// servers and datasources (load the app with App.Get, which preloads them):
//   - read_only when it has none of them (nor plugin resources);
//   - read_tools when every tool operation the app can call is read-only
//     (GET/HEAD/OPTIONS, or x-mcp-read-only: true on the operation), every
//     MCP tool primitive is marked readOnlyHint, and datasources (retrieval)
//     are read-only by nature;
//   - acting otherwise, including whatever Studio cannot tell (a spec it
//     cannot parse, an MCP server whose primitives are not known).
//
// Plugin resources bound to the App (pluginResources; load them with
// GetAppPluginResources, with their PluginResourceType) are something
// Studio cannot classify, so any binding makes it acting.
func SuggestAppCapability(app *models.App, pluginResources ...models.AppPluginResource) CapabilitySuggestion {
	var acting, reading []string

	for _, pr := range pluginResources {
		kind := fmt.Sprintf("resource type %d", pr.PluginResourceTypeID)
		if pr.PluginResourceType != nil {
			kind = fmt.Sprintf("%q of plugin %d", pr.PluginResourceType.Name, pr.PluginResourceType.PluginID)
		}
		name := pr.InstanceName
		if name == "" {
			name = pr.InstanceID
		}
		acting = append(acting, fmt.Sprintf("Plugin resource %q (%s): what it can do is not known to Studio", name, kind))
	}

	for i := range app.Tools {
		a, r := toolCapabilityReasons(&app.Tools[i])
		acting = append(acting, a...)
		reading = append(reading, r...)
	}
	for i := range app.MCPServers {
		a, r := mcpServerCapabilityReasons(&app.MCPServers[i])
		acting = append(acting, a...)
		reading = append(reading, r...)
	}
	for _, ds := range app.Datasources {
		reading = append(reading, fmt.Sprintf("Datasource %q: retrieval only", ds.Name))
	}

	switch {
	case len(acting) > 0:
		return CapabilitySuggestion{Suggestion: guardrails.CapabilityActing, Reasons: acting}
	case len(reading) > 0:
		return CapabilitySuggestion{Suggestion: guardrails.CapabilityReadTools, Reasons: reading}
	default:
		return CapabilitySuggestion{
			Suggestion: guardrails.CapabilityReadOnly,
			Reasons:    []string{"No tools, datasources or MCP servers attached"},
		}
	}
}

// toolCapabilityReasons classifies the operations of a tool the app can call.
// A tool the app cannot reach (a chat-only tool) adds nothing.
func toolCapabilityReasons(t *models.Tool) (acting, reading []string) {
	if !t.AppGrantable() {
		return nil, nil
	}
	ops := t.GetOperations()
	if len(ops) == 0 {
		return nil, []string{fmt.Sprintf("Tool %q: exposes no operations", t.Name)}
	}
	spec, err := base64.StdEncoding.DecodeString(t.OASSpec)
	if err != nil || len(spec) == 0 {
		return []string{fmt.Sprintf("Tool %q: its OpenAPI spec could not be read", t.Name)}, nil
	}
	client, err := universalclient.NewClient(spec, "")
	if err != nil {
		return []string{fmt.Sprintf("Tool %q: its OpenAPI spec could not be read", t.Name)}, nil
	}
	var readOps []string
	for _, op := range ops {
		op = strings.TrimSpace(op)
		if op == "" {
			continue
		}
		binding, err := client.GetOperationBinding(op)
		if err != nil {
			acting = append(acting, fmt.Sprintf("Tool %q: operation %s is not in its spec", t.Name, op))
			continue
		}
		ann := mcpannotations.ApplyOverrides(mcpannotations.ForMethod(binding.Method), binding.Extensions)
		if ann.ReadOnlyHint == nil || !*ann.ReadOnlyHint {
			acting = append(acting, fmt.Sprintf("Tool %q: operation %s (%s) is not read-only", t.Name, op, binding.Method))
			continue
		}
		readOps = append(readOps, op)
	}
	if len(acting) > 0 {
		return acting, nil
	}
	if len(readOps) == 0 {
		return nil, []string{fmt.Sprintf("Tool %q: exposes no operations", t.Name)}
	}
	return nil, []string{fmt.Sprintf("Tool %q: read-only operations only (%s)", t.Name, strings.Join(readOps, ", "))}
}

// mcpServerCapabilityReasons classifies an MCP server's known primitives.
// Resources and prompts only read; a tool reads only when it is annotated
// readOnlyHint: true (the MCP default is false).
func mcpServerCapabilityReasons(m *models.MCPServer) (acting, reading []string) {
	prims := m.Primitives()
	if len(prims) == 0 {
		return []string{fmt.Sprintf("MCP server %q: its primitives are not known", m.Name)}, nil
	}
	for _, p := range prims {
		if p.Type != "tool" {
			continue
		}
		if ro, _ := p.Annotations["readOnlyHint"].(bool); !ro {
			acting = append(acting, fmt.Sprintf("MCP server %q: tool %s is not marked read-only", m.Name, p.Name))
		}
	}
	if len(acting) > 0 {
		return acting, nil
	}
	return nil, []string{fmt.Sprintf("MCP server %q: read-only primitives only", m.Name)}
}

// SuggestAppCapabilityFor suggests a capability for an App loaded with
// App.Get, reading its plugin resources too.
func (s *Service) SuggestAppCapabilityFor(app *models.App) (CapabilitySuggestion, error) {
	prs, err := s.GetAppPluginResources(app.ID)
	if err != nil {
		return CapabilitySuggestion{}, err
	}
	return SuggestAppCapability(app, prs...), nil
}

// SetAppGuardrailCapability records the capability a full administrator
// attests for an app ("" clears it back to unattested). Only the column is
// written. The API restricts it to full administrators.
func (s *Service) SetAppGuardrailCapability(appID uint, value string, userID uint) (*models.App, error) {
	c, err := guardrails.ParseCapability(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidGuardrailCapability, err)
	}
	app, err := s.GetAppByID(appID)
	if err != nil {
		return nil, err
	}
	if app.GuardrailCapability == string(c) && app.GuardrailCapabilityResetAt == nil {
		return app, nil
	}
	// Attesting (or clearing) by hand also clears the record of the last
	// reset: it has been dealt with.
	if err := s.DB.Model(&models.App{}).Where("id = ?", appID).Updates(map[string]any{
		"guardrail_capability":          string(c),
		"guardrail_capability_reset_by": nil,
		"guardrail_capability_reset_at": nil,
	}).Error; err != nil {
		return nil, err
	}
	app.GuardrailCapabilityResetBy, app.GuardrailCapabilityResetAt = nil, nil
	// The runtime reads the capability with the exceptions it bounds.
	s.GuardrailExceptionSource().forget(appID)
	app.GuardrailCapability = string(c)
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppUpdated(app, app.ID, userID)
	}
	return app, nil
}

// Capability drift (features/GuardrailExceptions.md): the capability is an
// attestation of what the App can do with its tools, datasources, MCP
// servers and plugin resources. When anyone other than a full administrator
// changes those bindings on an attested App, the attestation no longer
// holds and is cleared (unattested is treated as acting) until a full
// administrator attests again. It is enforced here, in the service, so every
// caller is covered: the admin API, the management gRPC API plugins use, and
// anything else. A caller that does not name its actor (WithCapabilityActor)
// counts as not a full administrator.

type capabilityActor struct {
	userID    uint
	fullAdmin bool
}

// WithCapabilityActor names who changes an App's resources: a full
// administrator's change keeps the attested guardrail capability; anyone
// else's clears it.
func WithCapabilityActor(userID uint, fullAdmin bool) AppOption {
	return func(o *appOptions) {
		o.capabilityActor = &capabilityActor{userID: userID, fullAdmin: fullAdmin}
	}
}

// appCapabilityResources fingerprints an App's capability-relevant
// bindings: its tools, datasources, MCP servers and plugin resources.
func (s *Service) appCapabilityResources(appID uint) (string, error) {
	var b strings.Builder
	for _, j := range []struct{ table, column string }{
		{"app_tools", "tool_id"},
		{"app_datasources", "datasource_id"},
		{"app_mcp_servers", "mcp_server_id"},
	} {
		var ids []uint
		if err := s.DB.Table(j.table).Where("app_id = ?", appID).Order(j.column).Pluck(j.column, &ids).Error; err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s:%v;", j.table, ids)
	}
	var prs []models.AppPluginResource
	if err := s.DB.Select("plugin_resource_type_id", "instance_id").Where("app_id = ?", appID).
		Order("plugin_resource_type_id, instance_id").Find(&prs).Error; err != nil {
		return "", err
	}
	b.WriteString("plugin_resources:")
	for _, pr := range prs {
		fmt.Fprintf(&b, "%d/%q,", pr.PluginResourceTypeID, pr.InstanceID)
	}
	return b.String(), nil
}

// guardCapabilityDrift runs change, a change to the App's resources, and
// clears the App's attested guardrail capability when the change altered
// them and the actor is not a full administrator. It reports whether it
// cleared it visibly (resetGuardrailCapability). The check runs even when
// change fails, since a failed change may have altered some bindings.
func (s *Service) guardCapabilityDrift(appID uint, o appOptions, change func() error) (bool, error) {
	actor := o.capabilityActor
	if actor != nil && actor.fullAdmin {
		return false, change()
	}
	var current models.App
	if err := s.DB.Select("id", "guardrail_capability").First(&current, appID).Error; err != nil || current.GuardrailCapability == "" {
		// Unattested (or not found: change reports it): nothing to clear.
		return false, change()
	}
	before, ferr := s.appCapabilityResources(appID)
	cerr := change()
	after, aerr := s.appCapabilityResources(appID)
	if ferr == nil && aerr == nil && before == after {
		return false, cerr
	}
	// Changed, or unknown because a fingerprint failed: clear it.
	var by *uint
	if actor != nil && actor.userID != 0 {
		by = &actor.userID
	}
	visible, err := s.resetGuardrailCapability(appID, by)
	if err != nil {
		// Fail closed: the resources changed, so the attestation must not
		// survive. Try once more, then report the failure so the caller
		// does not answer as if all went well.
		slog.Warn("could not clear an app's guardrail capability after a change to its resources; retrying", "app_id", appID, "error", err)
		if visible, err = s.resetGuardrailCapability(appID, by); err != nil {
			slog.Error("could not clear an app's guardrail capability after a change to its resources", "app_id", appID, "error", err)
			return false, errors.Join(cerr, fmt.Errorf("%w: %v", ErrGuardrailCapabilityReset, err))
		}
	}
	return visible, cerr
}

// ErrGuardrailCapabilityReset: an App's resources changed but its attested
// guardrail capability could not be cleared. The change itself may have
// been saved; the caller reports a failure.
var ErrGuardrailCapabilityReset = errors.New("the app's resources changed but its guardrail capability could not be cleared")

// GuardrailsApplyToApp reports whether a guardrail filter is attached to
// at least one of the App's LLMs (in any mode).
func (s *Service) GuardrailsApplyToApp(appID uint) (bool, error) {
	var n int64
	err := s.DB.Table("app_llms").
		Joins("JOIN llm_filters ON llm_filters.llm_id = app_llms.llm_id").
		Joins("JOIN filters ON filters.id = llm_filters.filter_id").
		Where("app_llms.app_id = ? AND filters.kind = ? AND filters.deleted_at IS NULL", appID, models.FilterKindGuardrail).
		Count(&n).Error
	return n > 0, err
}

// resetGuardrailCapability clears an App's attested capability after a
// change to its resources. Where guardrails apply to the App the reset is
// visible: who and when are recorded and the full administrators are asked
// to attest again. Otherwise it is quiet: the attestation has no effect
// without guardrails, and an App that gains one later starts at the
// strictest tier and shows the suggestion. It reports a visible reset.
func (s *Service) resetGuardrailCapability(appID uint, by *uint) (bool, error) {
	visible, err := s.GuardrailsApplyToApp(appID)
	if err != nil {
		// Unknown: make it visible rather than lose it.
		visible = true
	}
	updates := map[string]any{
		"guardrail_capability":          "",
		"guardrail_capability_reset_by": nil,
		"guardrail_capability_reset_at": nil,
	}
	if visible {
		updates["guardrail_capability_reset_by"] = by
		updates["guardrail_capability_reset_at"] = time.Now()
	}
	res := s.DB.Model(&models.App{}).Where("id = ? AND guardrail_capability <> ''", appID).Updates(updates)
	if res.Error != nil || res.RowsAffected == 0 {
		return false, res.Error
	}
	s.GuardrailExceptionSource().forget(appID)
	if s.SystemEvents != nil {
		s.SystemEvents.EmitAppUpdated(nil, appID, derefUint(by))
	}
	if visible {
		s.notifyGuardrailCapabilityReset(appID, by)
	}
	return visible, nil
}

func derefUint(p *uint) uint {
	if p == nil {
		return 0
	}
	return *p
}

// notifyGuardrailCapabilityReset asks the full administrators to attest the
// App's capability again.
func (s *Service) notifyGuardrailCapabilityReset(appID uint, by *uint) {
	if s.NotificationService == nil {
		return
	}
	var app models.App
	if err := s.DB.Select("id", "name").First(&app, appID).Error; err != nil {
		return
	}
	link := fmt.Sprintf("/admin/apps/%d", appID)
	content := fmt.Sprintf("The tools, datasources, MCP servers or plugin resources of the app **%s** changed, so its attested guardrail capability was cleared. "+
		"Until a full administrator attests it again it is treated as acting (the strictest), and guardrail exceptions for it may block again.\n\n[Re-attest it on the app page](%s)",
		EscapeMarkdown(app.Name), link)
	if err := s.NotificationService.NotifyWithOptions(
		fmt.Sprintf("guardrail_capability_reset_%d_%d", appID, time.Now().UnixNano()),
		fmt.Sprintf("Re-attest the guardrail capability of %s", app.Name),
		content, models.NotifyAdmins,
		NotifyOptions{Type: "guardrail_capability", Link: link, ActorID: derefUint(by)},
	); err != nil {
		slog.Warn("could not notify administrators of a guardrail capability reset", "app_id", appID, "error", err)
	}
}
