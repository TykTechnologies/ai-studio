package tykmcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Community registration: a portal user submits an MCP server through the
// submission pipeline (resource_type "mcp_server"); on approval Studio either
// creates the proxy on a full-mode connection or hands a package to the
// platform team on a catalogue/broker connection.

// SubmissionRegistration carries an approved submission to the Dashboard.
type SubmissionRegistration struct {
	SubmissionID uint
	SubmitterID  uint
	Input        RegisterInput
}

// SubmissionConnection is a connection a portal user may submit to.
type SubmissionConnection struct {
	ID                 uint               `json:"id"`
	Name               string             `json:"name"`
	EffectiveMode      string             `json:"effective_mode"`
	DirectCreate       bool               `json:"direct_create"` // full mode: approval creates the proxy
	AcceptsHandoffs    bool               `json:"accepts_handoffs"`
	RestToMCPSupported bool               `json:"rest_to_mcp_supported"`
	GatewayTags        []GatewayTagOption `json:"gateway_tags"`
}

// HandoffContact identifies who asked for the proxy.
type HandoffContact struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	PrimaryContact string `json:"primary_contact,omitempty"`
}

// HandoffPackage is what a platform team needs to create the proxy by hand
// on a connection where Studio may not write.
type HandoffPackage struct {
	Server               models.MCPServerResponse   `json:"server"`
	SubmissionID         uint                       `json:"submission_id"`
	Submitter            HandoffContact             `json:"submitter"`
	Definition           json.RawMessage            `json:"definition"`
	SecretsIncluded      bool                       `json:"secrets_included"`
	RequestedGatewayTags []string                   `json:"requested_gateway_tags"`
	TransportNotes       string                     `json:"transport_notes,omitempty"`
	PolicyShape          map[string]interface{}     `json:"policy_shape"`
	Instructions         []string                   `json:"instructions"`
	Candidates           []models.MCPServerResponse `json:"candidates"`
}

// SubmissionPayloadSchema is the JSON schema the portal form and the API
// validate an mcp_server submission payload against.
const SubmissionPayloadSchema = `{
  "type": "object",
  "required": ["name", "description", "kind", "connection_id"],
  "properties": {
    "name": {"type": "string"},
    "description": {"type": "string"},
    "long_description": {"type": "string"},
    "kind": {"enum": ["remote", "rest_to_mcp"]},
    "connection_id": {"type": "integer"},
    "upstream_url": {"type": "string", "format": "uri"},
    "upstream_auth_header_name": {"type": "string"},
    "upstream_auth_token": {"type": "string"},
    "allowed_tools": {"type": "array", "items": {"type": "string"}},
    "source_api_id": {"type": "string"},
    "primitives": {"type": "array"},
    "consumer_auth": {"enum": ["auth_token", "oauth21", "keyless"]},
    "authorization_servers": {"type": "array", "items": {"type": "string", "format": "uri"}},
    "scopes_supported": {"type": "array", "items": {"type": "string"}},
    "confirm_keyless": {"type": "boolean"},
    "transport_notes": {"type": "string"},
    "suggested_listen_path": {"type": "string"},
    "gateway_tags": {"type": "array", "items": {"type": "string"}},
    "confirm_no_gateway_tags": {"type": "boolean"},
    "tags": {"type": "array", "items": {"type": "string"}}
  }
}`

func payloadString(p map[string]interface{}, key string) string {
	if v, ok := p[key]; ok {
		switch t := v.(type) {
		case string:
			return strings.TrimSpace(t)
		case float64:
			return fmt.Sprint(int64(t))
		}
	}
	return ""
}

func payloadBool(p map[string]interface{}, key string) bool {
	b, _ := p[key].(bool)
	return b
}

func payloadUint(p map[string]interface{}, key string) uint {
	switch t := p[key].(type) {
	case float64:
		if t > 0 {
			return uint(t)
		}
	case int:
		if t > 0 {
			return uint(t)
		}
	case string:
		var n uint
		if _, err := fmt.Sscanf(strings.TrimSpace(t), "%d", &n); err == nil {
			return n
		}
	}
	return 0
}

func payloadStrings(p map[string]interface{}, key string) []string {
	var out []string
	switch t := p[key].(type) {
	case []interface{}:
		for _, e := range t {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case []string:
		out = append(out, t...)
	case string:
		for _, s := range strings.Split(t, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// RegisterInputFromPayload maps an mcp_server submission payload onto the
// registration input the enterprise service renders. The payload's
// suggested_listen_path is a suggestion: the reviewer may change it.
func RegisterInputFromPayload(p map[string]interface{}) RegisterInput {
	in := RegisterInput{
		ConnectionID:           payloadUint(p, "connection_id"),
		Kind:                   payloadString(p, "kind"),
		Name:                   payloadString(p, "name"),
		ListenPath:             payloadString(p, "suggested_listen_path"),
		UpstreamURL:            payloadString(p, "upstream_url"),
		UpstreamAuthHeaderName: payloadString(p, "upstream_auth_header_name"),
		UpstreamAuthToken:      payloadString(p, "upstream_auth_token"),
		AllowedTools:           payloadStrings(p, "allowed_tools"),
		SourceAPIID:            payloadString(p, "source_api_id"),
		ConsumerAuth:           payloadString(p, "consumer_auth"),
		AuthorizationServers:   payloadStrings(p, "authorization_servers"),
		ScopesSupported:        payloadStrings(p, "scopes_supported"),
		ConfirmKeyless:         payloadBool(p, "confirm_keyless"),
		GatewayTags:            payloadStrings(p, "gateway_tags"),
		ConfirmNoGatewayTags:   payloadBool(p, "confirm_no_gateway_tags"),
		Description:            payloadString(p, "description"),
		LongDescription:        payloadString(p, "long_description"),
		Tags:                   payloadStrings(p, "tags"),
	}
	if in.UpstreamAuthToken == "[redacted]" {
		in.UpstreamAuthToken = ""
	}
	if prims, ok := p["primitives"].([]interface{}); ok {
		for _, e := range prims {
			m, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			rp := RegisterPrimitive{
				OperationID: payloadString(m, "operation_id"),
				Method:      payloadString(m, "method"),
				Path:        payloadString(m, "path"),
				Name:        payloadString(m, "name"),
				Description: payloadString(m, "description"),
			}
			if ann, ok := m["annotations"].(map[string]interface{}); ok {
				rp.Annotations = ann
			}
			in.Primitives = append(in.Primitives, rp)
		}
	}
	return in
}
