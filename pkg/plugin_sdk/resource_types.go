package plugin_sdk

import (
	"context"
	"encoding/json"
	"fmt"
)

// ResourceTypeRegistration declares a resource type provided by a plugin.
// Each plugin can register one or more resource types that appear in the
// App creation/editing flow and participate in the governance model.
type ResourceTypeRegistration struct {
	// Slug is the machine-readable identifier (e.g., "mcp_servers").
	// Must be unique per plugin. Used in API paths and DB storage.
	Slug string

	// Name is the human-readable display name (e.g., "MCP Servers").
	Name string

	// Description explains what this resource type is.
	Description string

	// Icon is an optional icon identifier (Material icon name or asset path).
	Icon string

	// HasPrivacyScore indicates whether instances carry a privacy score.
	// If true, instances' privacy scores are subject to the generalized
	// privacy rule: no resource score may exceed the max LLM score in the app.
	HasPrivacyScore bool

	// SupportsSubmissions indicates whether community users can submit
	// new instances of this resource type through the submission workflow.
	SupportsSubmissions bool

	// SupportsMetadata indicates whether instances of this resource type can
	// carry governed metadata (Enterprise). When true, admins can attach
	// schema-validated metadata to instances under the object type
	// "plugin_resource:<plugin_id>:<slug>". Must also be set as
	// "supports_metadata" on the resource type in manifest.json.
	SupportsMetadata bool

	// FormComponent declares how the plugin provides its App Form UI section.
	// If nil, the platform renders a standard multi-select populated via ListResourceInstances.
	FormComponent *ResourceFormComponent

	// SubmissionSchema is an optional JSON Schema (with "type": "object")
	// describing the payload of community submissions for this type. When set,
	// the portal renders the submission form from it and the platform validates
	// submitted payloads against it before accepting them. Only meaningful when
	// SupportsSubmissions is true.
	SubmissionSchema string
}

// ResourceFormComponent declares a Web Component that the platform will render
// inside the App Create/Edit form for resource selection.
type ResourceFormComponent struct {
	// Tag is the custom element tag name (e.g., "mcp-server-selector").
	Tag string

	// EntryPoint is the path to the JS file relative to plugin assets
	// (e.g., "ui/webc/mcp-selector.js").
	EntryPoint string
}

// ResourceInstance represents a single instance of a plugin resource type.
// Instances are managed by the plugin (stored in plugin KV or external systems)
// and exposed to the platform via the ResourceProvider interface.
type ResourceInstance struct {
	// ID is the plugin-assigned unique identifier for this instance.
	// Stored as a string to accommodate any ID format.
	ID string

	// Name is the human-readable display name.
	Name string

	// Description is an optional description.
	Description string

	// PrivacyScore is the privacy score (0-100). Only meaningful
	// if the ResourceTypeRegistration has HasPrivacyScore=true.
	PrivacyScore int

	// Metadata is arbitrary plugin-defined metadata as JSON bytes.
	// This is opaque to the platform but included in config snapshots.
	//
	// SECURITY: Do NOT store secrets, credentials, or PII in this field.
	// Metadata is propagated to all gateways, cached in the join table,
	// and may appear in logs or be accessible to other plugins.
	Metadata []byte

	// IsActive indicates if this instance is currently usable.
	IsActive bool
}

// ResourceInstanceChangedEvent is the topic for instance change notifications.
const ResourceInstanceChangedEvent = "system.plugin_resource.instance_changed"

// resourceInstanceChangedPayload is the event payload for instance change notifications.
type resourceInstanceChangedPayload struct {
	ResourceTypeSlug string `json:"resource_type_slug"`
	InstanceID       string `json:"instance_id"`
}

// NotifyResourceInstanceChanged publishes an event telling the platform that a
// resource instance's data (name, privacy score, metadata, active status) has changed.
// The platform subscribes to this event and refreshes cached instance details in all
// AppPluginResource rows that reference this instance.
//
// Call this from your plugin whenever you update an instance's properties.
func NotifyResourceInstanceChanged(ctx Context, resourceTypeSlug, instanceID string) error {
	eventSvc := ctx.Services.Events()
	if eventSvc == nil {
		return fmt.Errorf("event service not available")
	}

	payload := resourceInstanceChangedPayload{
		ResourceTypeSlug: resourceTypeSlug,
		InstanceID:       instanceID,
	}

	return eventSvc.Publish(ctx.Context, ResourceInstanceChangedEvent, payload, DirLocal)
}

// SubmissionUser identifies a user involved in a community submission.
type SubmissionUser struct {
	ID    uint32 `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// SubmissionEnvelope is the payload AI Studio passes to
// ResourceProvider.CreateResourceInstance when an administrator approves a
// community submission for one of the plugin's resource types.
//
// The RPC is issued outside the platform's database transaction, so a plugin
// must treat SubmissionID as an idempotency key: if an instance already exists
// for the same submission, return it instead of creating a duplicate.
type SubmissionEnvelope struct {
	Source               string                 `json:"source"` // always "submission"
	SubmissionID         uint32                 `json:"submission_id"`
	Submitter            SubmissionUser         `json:"submitter"`
	Reviewer             SubmissionUser         `json:"reviewer"`
	FinalPrivacyScore    int                    `json:"final_privacy_score"`
	SuggestedPrivacy     int                    `json:"suggested_privacy"`
	PrivacyJustification string                 `json:"privacy_justification"`
	AssignedCatalogues   []uint32               `json:"assigned_catalogues"`
	DocumentationURL     string                 `json:"documentation_url"`
	Notes                string                 `json:"notes"`
	Attestations         interface{}            `json:"attestations"`
	ResourcePayload      map[string]interface{} `json:"resource_payload"`
}

// ParseSubmissionEnvelope decodes the payload passed to CreateResourceInstance.
// Payloads without a "source" field are treated as a bare resource payload
// (the pre-envelope contract), so plugins can accept both shapes.
func ParseSubmissionEnvelope(payload []byte) (*SubmissionEnvelope, error) {
	var env SubmissionEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, fmt.Errorf("parse submission envelope: %w", err)
	}
	if env.Source == "" {
		var bare map[string]interface{}
		if err := json.Unmarshal(payload, &bare); err != nil {
			return nil, fmt.Errorf("parse submission payload: %w", err)
		}
		env = SubmissionEnvelope{ResourcePayload: bare}
	}
	if env.ResourcePayload == nil {
		env.ResourcePayload = map[string]interface{}{}
	}
	return &env, nil
}

// SyncResourceTypes (re)registers the plugin's resource types with AI Studio at
// runtime and deactivates any of the plugin's previously registered types that
// are not in regs. Use it when the set of types is defined dynamically (for
// example by administrators inside the plugin) rather than in the manifest.
//
// Requires the "resource-types.manage" service scope. Studio-only.
//
// The platform calls back ListResourceInstances for every registered type
// while this call is in flight, so do not hold locks that ListResourceInstances
// needs across the call.
func SyncResourceTypes(ctx Context, regs []*ResourceTypeRegistration) error {
	if ctx.Runtime != RuntimeStudio {
		return fmt.Errorf("resource types can only be registered in the Studio runtime")
	}
	if ctx.Services == nil {
		return fmt.Errorf("services not available")
	}
	studio := ctx.Services.Studio()
	if studio == nil {
		return fmt.Errorf("studio services not available")
	}
	specs := make([]ResourceTypeRegistration, 0, len(regs))
	for _, r := range regs {
		if r != nil {
			specs = append(specs, *r)
		}
	}
	base := ctx.Context
	if base == nil {
		base = context.Background()
	}
	_, _, err := studio.RegisterResourceTypes(base, specs, true)
	return err
}
