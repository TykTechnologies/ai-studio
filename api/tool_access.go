package api

import (
	"errors"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// errClientToolAccess is returned when a caller asks for a gateway access
// method on a client tool, which runs in the chat UI and has no endpoint.
var errClientToolAccess = errors.New("client tools run in the chat UI and cannot be reached over REST or MCP")

// toolGatewayBaseURL is where the gateway serves /tools/, as the portal should
// show it: TOOL_DISPLAY_URL, falling back to PROXY_URL. Empty when neither is
// configured.
func toolGatewayBaseURL() string {
	cfg := config.Get("")
	base := cfg.ToolDisplayURL
	if base == "" {
		base = cfg.ProxyURL
	}
	return strings.TrimRight(base, "/")
}

// toolGatewayAccess builds the access-method block of a tool response.
func toolGatewayAccess(tool *models.Tool) ToolGatewayAccess {
	access := ToolGatewayAccess{
		Slug:              tool.Slug,
		RESTAccessEnabled: tool.RESTAccessEnabled(),
		MCPAccessEnabled:  tool.MCPAccessEnabled(),
		AppGrantable:      tool.AppGrantable(),
	}
	if tool.ToolType == models.ToolTypeClient || tool.Slug == "" {
		return access
	}
	if base := toolGatewayBaseURL(); base != "" {
		access.RESTEndpointURL = base + "/tools/" + tool.Slug
		access.MCPEndpointURL = access.RESTEndpointURL + "/mcp"
	}
	return access
}

// emitToolUpdated announces a tool change made by a handler that writes the
// model directly. Tools are part of the edge configuration snapshot, so the
// control server listens for this to mark edge gateways as pending.
func (a *API) emitToolUpdated(tool *models.Tool) {
	if a.service != nil && a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitToolUpdated(tool, tool.ID, 0)
	}
}

// applyToolAccessInput writes the access-method switches of a tool input onto
// the tool. A nil switch leaves the stored value alone: on create that is the
// default set by the service layer, on update the current value.
func applyToolAccessInput(tool *models.Tool, restEnabled, mcpEnabled *bool) error {
	if tool.ToolType == models.ToolTypeClient {
		if (restEnabled != nil && *restEnabled) || (mcpEnabled != nil && *mcpEnabled) {
			return errClientToolAccess
		}
		return nil
	}
	if restEnabled != nil {
		tool.RESTAccessDisabled = !*restEnabled
	}
	if mcpEnabled != nil {
		tool.MCPAccessDisabled = !*mcpEnabled
	}
	return nil
}
