package ai_studio_sdk

import (
	"context"
	"fmt"

	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

// Read-only governance APIs (metadata history, audit trail, MCP servers,
// routers) and team grants on the plugin's own resource instances.

// ListObjectMetadataAudit returns the governed metadata audit trail of an
// object, newest first (limit 0 = 100, capped at 500). objectType accepts
// "plugin_resource:self:<slug>". Community Edition returns an empty list.
// Requires the metadata.read scope.
func ListObjectMetadataAudit(ctx context.Context, objectType, objectID string, limit int32) (*mgmtpb.ListObjectMetadataAuditResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.ListObjectMetadataAudit(ctx, &mgmtpb.ListObjectMetadataAuditRequest{
		Context:    createPluginContext(AvailableScopes.MetadataRead),
		ObjectType: objectType,
		ObjectId:   objectID,
		Limit:      limit,
	})
}

// ListAuditRecords returns the platform audit records for one resource
// ("llm", "tool", "datasource", "mcp_server", "app", ...), newest first,
// without request or response bodies. Enterprise only (Unimplemented in
// Community Edition). Requires the audit.read scope.
func ListAuditRecords(ctx context.Context, resourceType, resourceID string, limit int32, mutationsOnly bool) (*mgmtpb.ListAuditRecordsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.ListAuditRecords(ctx, &mgmtpb.ListAuditRecordsRequest{
		Context:       createPluginContext(AvailableScopes.AuditRead),
		ResourceType:  resourceType,
		ResourceId:    resourceID,
		Limit:         limit,
		MutationsOnly: mutationsOnly,
	})
}

// ListMCPServers lists MCP servers (never upstream URLs or auth details).
// Requires the mcp-servers.read scope.
func ListMCPServers(ctx context.Context, page, limit int32, isActive *bool) (*mgmtpb.ListMCPServersResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.ListMCPServers(ctx, &mgmtpb.ListMCPServersRequest{
		Context:  createPluginContext(AvailableScopes.MCPServersRead),
		Page:     page,
		Limit:    limit,
		IsActive: isActive,
	})
}

// GetMCPServer returns one MCP server. Requires the mcp-servers.read scope.
func GetMCPServer(ctx context.Context, serverID uint32) (*mgmtpb.GetMCPServerResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.GetMCPServer(ctx, &mgmtpb.GetMCPServerRequest{
		Context:  createPluginContext(AvailableScopes.MCPServersRead),
		ServerId: serverID,
	})
}

// ListModelRouters lists model routers with the LLMs each can route to.
// Requires the routers.read scope.
func ListModelRouters(ctx context.Context, page, limit int32) (*mgmtpb.ListModelRoutersResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.ListModelRouters(ctx, &mgmtpb.ListModelRoutersRequest{
		Context: createPluginContext(AvailableScopes.RoutersRead),
		Page:    page,
		Limit:   limit,
	})
}

// GetModelRouter returns one model router. Requires the routers.read scope.
func GetModelRouter(ctx context.Context, routerID uint32) (*mgmtpb.GetModelRouterResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.GetModelRouter(ctx, &mgmtpb.GetModelRouterRequest{
		Context:  createPluginContext(AvailableScopes.RoutersRead),
		RouterId: routerID,
	})
}

// ListSemanticRouters lists semantic routers with their targets.
// Requires the routers.read scope.
func ListSemanticRouters(ctx context.Context, page, limit int32) (*mgmtpb.ListSemanticRoutersResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.ListSemanticRouters(ctx, &mgmtpb.ListSemanticRoutersRequest{
		Context: createPluginContext(AvailableScopes.RoutersRead),
		Page:    page,
		Limit:   limit,
	})
}

// GetSemanticRouter returns one semantic router. Requires the routers.read scope.
func GetSemanticRouter(ctx context.Context, routerID uint32) (*mgmtpb.GetSemanticRouterResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.GetSemanticRouter(ctx, &mgmtpb.GetSemanticRouterRequest{
		Context:  createPluginContext(AvailableScopes.RoutersRead),
		RouterId: routerID,
	})
}

// ListGroups lists every team, with the Default team flagged.
// Requires the resource-access.manage scope.
func ListGroups(ctx context.Context) (*mgmtpb.ListGroupsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.ListGroups(ctx, &mgmtpb.ListGroupsRequest{
		Context: createPluginContext(AvailableScopes.ResourceAccessManage),
	})
}

// GetResourceInstanceGroups returns the teams granted one of the plugin's
// resource instances. Requires the resource-access.manage scope.
func GetResourceInstanceGroups(ctx context.Context, resourceTypeSlug, instanceID string) (*mgmtpb.GetResourceInstanceGroupsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.GetResourceInstanceGroups(ctx, &mgmtpb.GetResourceInstanceGroupsRequest{
		Context:          createPluginContext(AvailableScopes.ResourceAccessManage),
		ResourceTypeSlug: resourceTypeSlug,
		InstanceId:       instanceID,
	})
}

// SetResourceInstanceGroups adds (replace=false) or sets (replace=true) the
// teams granted one of the plugin's resource instances. Requires the
// resource-access.manage scope.
func SetResourceInstanceGroups(ctx context.Context, resourceTypeSlug, instanceID string, groupIDs []uint32, replace bool) (*mgmtpb.SetResourceInstanceGroupsResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.SetResourceInstanceGroups(ctx, &mgmtpb.SetResourceInstanceGroupsRequest{
		Context:          createPluginContext(AvailableScopes.ResourceAccessManage),
		ResourceTypeSlug: resourceTypeSlug,
		InstanceId:       instanceID,
		GroupIds:         groupIDs,
		Replace:          replace,
	})
}

// ListAccessibleResourceInstances returns which of the plugin's instances of
// a type a user can reach under the platform's portal rule. Requires the
// resource-access.manage scope.
func ListAccessibleResourceInstances(ctx context.Context, resourceTypeSlug string, userID uint32) (*mgmtpb.ListAccessibleResourceInstancesResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.ListAccessibleResourceInstances(ctx, &mgmtpb.ListAccessibleResourceInstancesRequest{
		Context:          createPluginContext(AvailableScopes.ResourceAccessManage),
		ResourceTypeSlug: resourceTypeSlug,
		UserId:           userID,
	})
}

// SetAppGovernanceState suspends or reactivates an App (isActive nil leaves
// it alone) and raises or clears governance flags on it (an empty value
// clears a flag). Requires the apps.lifecycle scope.
func SetAppGovernanceState(ctx context.Context, appID uint32, isActive *bool, flags map[string]string, reason string) (*mgmtpb.SetAppGovernanceStateResponse, error) {
	client, err := getServiceClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("service client unavailable: %w", err)
	}
	return client.SetAppGovernanceState(ctx, &mgmtpb.SetAppGovernanceStateRequest{
		Context:  createPluginContext(AvailableScopes.AppsLifecycle),
		AppId:    appID,
		IsActive: isActive,
		Flags:    flags,
		Reason:   reason,
	})
}
