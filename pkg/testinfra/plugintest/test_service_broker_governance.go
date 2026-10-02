package plugintest

import (
	"context"
	"sort"
	"strings"
	"sync"

	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Fakes for the governance read RPCs (audit trail, MCP servers, routers) and
// the team-grant RPCs a ResourceProvider plugin uses on its own instances.
//
// Configure the core objects a plugin should see with AddMCPServer,
// AddModelRouter, AddSemanticRouter and AddAuditRecord; configure teams with
// AddGroup and SetUserGroups (SetUserSeesAll for a team manager). Grants are
// limited, as in Studio, to resource types the plugin registered through
// RegisterResourceTypes. SetAuditEnterprise(false) makes ListAuditRecords
// answer Unimplemented, as Community Edition does.

type governanceState struct {
	mu              sync.RWMutex
	auditDisabled   bool
	auditRecords    []*mgmtpb.AuditRecordInfo
	mcpServers      []*mgmtpb.MCPServerInfo
	modelRouters    []*mgmtpb.ModelRouterInfo
	semanticRouters []*mgmtpb.SemanticRouterInfo
	groups          []*mgmtpb.GroupInfo
	grants          map[string]map[uint32]bool // slug|instance → group IDs
	userGroups      map[uint32]map[uint32]bool
	userSeesAll     map[uint32]bool
	apps            map[uint32]*mgmtpb.AppInfo
	appFlags        map[uint32]map[string]string
}

func (s *TestManagementServer) governance() *governanceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.governanceState == nil {
		s.governanceState = &governanceState{
			grants:      map[string]map[uint32]bool{},
			userGroups:  map[uint32]map[uint32]bool{},
			userSeesAll: map[uint32]bool{},
			apps:        map[uint32]*mgmtpb.AppInfo{},
			appFlags:    map[uint32]map[string]string{},
		}
	}
	return s.governanceState
}

func (s *TestManagementServer) recordCall(method string, req interface{}) {
	s.recordMetadataCall(method, req)
}

// SetAuditEnterprise toggles ListAuditRecords between Enterprise (records)
// and Community Edition (Unimplemented). Enterprise by default.
func (s *TestManagementServer) SetAuditEnterprise(enabled bool) {
	g := s.governance()
	g.mu.Lock()
	g.auditDisabled = !enabled
	g.mu.Unlock()
}

// AddAuditRecord adds a platform audit record ListAuditRecords can return.
func (s *TestManagementServer) AddAuditRecord(r *mgmtpb.AuditRecordInfo) {
	g := s.governance()
	g.mu.Lock()
	g.auditRecords = append(g.auditRecords, r)
	g.mu.Unlock()
}

// AddMCPServer adds an MCP server the plugin can read.
func (s *TestManagementServer) AddMCPServer(m *mgmtpb.MCPServerInfo) {
	g := s.governance()
	g.mu.Lock()
	g.mcpServers = append(g.mcpServers, m)
	g.mu.Unlock()
}

// AddModelRouter adds a model router the plugin can read.
func (s *TestManagementServer) AddModelRouter(r *mgmtpb.ModelRouterInfo) {
	g := s.governance()
	g.mu.Lock()
	g.modelRouters = append(g.modelRouters, r)
	g.mu.Unlock()
}

// AddSemanticRouter adds a semantic router the plugin can read.
func (s *TestManagementServer) AddSemanticRouter(r *mgmtpb.SemanticRouterInfo) {
	g := s.governance()
	g.mu.Lock()
	g.semanticRouters = append(g.semanticRouters, r)
	g.mu.Unlock()
}

// AddGroup adds a team. Name "Default" marks it as the Default team.
func (s *TestManagementServer) AddGroup(id uint32, name string) {
	g := s.governance()
	g.mu.Lock()
	g.groups = append(g.groups, &mgmtpb.GroupInfo{Id: id, Name: name, IsDefault: name == "Default"})
	g.mu.Unlock()
}

// SetUserGroups sets the teams a user belongs to.
func (s *TestManagementServer) SetUserGroups(userID uint32, groupIDs ...uint32) {
	g := s.governance()
	g.mu.Lock()
	set := map[uint32]bool{}
	for _, id := range groupIDs {
		set[id] = true
	}
	g.userGroups[userID] = set
	g.mu.Unlock()
}

// SetUserSeesAll marks a user as a team manager who sees every instance.
func (s *TestManagementServer) SetUserSeesAll(userID uint32, seesAll bool) {
	g := s.governance()
	g.mu.Lock()
	g.userSeesAll[userID] = seesAll
	g.mu.Unlock()
}

// InstanceGroups returns the teams granted an instance, ascending.
func (s *TestManagementServer) InstanceGroups(slug, instanceID string) []uint32 {
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	return sortedGroupIDs(g.grants[slug+"|"+instanceID])
}

func sortedGroupIDs(set map[uint32]bool) []uint32 {
	out := make([]uint32, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ownsType reports whether the plugin registered the resource type.
func (s *TestManagementServer) ownsType(slug string) bool {
	for _, t := range s.GetResourceTypes() {
		if t.GetSlug() == slug {
			return true
		}
	}
	return false
}

func clampFake(limit int32, def int) int {
	if limit <= 0 {
		return def
	}
	return int(limit)
}

// ListAuditRecords implements the ListAuditRecords RPC.
func (s *TestManagementServer) ListAuditRecords(ctx context.Context, req *mgmtpb.ListAuditRecordsRequest) (*mgmtpb.ListAuditRecordsResponse, error) {
	s.recordCall("ListAuditRecords", req)
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.auditDisabled {
		return nil, status.Error(codes.Unimplemented, "the audit trail is an Enterprise Edition feature")
	}
	limit := clampFake(req.Limit, 50)
	out := []*mgmtpb.AuditRecordInfo{}
	for i := len(g.auditRecords) - 1; i >= 0 && len(out) < limit; i-- {
		r := g.auditRecords[i]
		if r.ResourceType != req.ResourceType || r.ResourceId != req.ResourceId {
			continue
		}
		if req.MutationsOnly && strings.EqualFold(r.Method, "GET") {
			continue
		}
		out = append(out, r)
	}
	return &mgmtpb.ListAuditRecordsResponse{Records: out}, nil
}

// ListMCPServers implements the ListMCPServers RPC (no paging).
func (s *TestManagementServer) ListMCPServers(ctx context.Context, req *mgmtpb.ListMCPServersRequest) (*mgmtpb.ListMCPServersResponse, error) {
	s.recordCall("ListMCPServers", req)
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	return &mgmtpb.ListMCPServersResponse{Servers: g.mcpServers, TotalCount: int64(len(g.mcpServers))}, nil
}

// GetMCPServer implements the GetMCPServer RPC.
func (s *TestManagementServer) GetMCPServer(ctx context.Context, req *mgmtpb.GetMCPServerRequest) (*mgmtpb.GetMCPServerResponse, error) {
	s.recordCall("GetMCPServer", req)
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, m := range g.mcpServers {
		if m.Id == req.ServerId {
			return &mgmtpb.GetMCPServerResponse{Server: m}, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "MCP server %d not found", req.ServerId)
}

// ListModelRouters implements the ListModelRouters RPC (no paging).
func (s *TestManagementServer) ListModelRouters(ctx context.Context, req *mgmtpb.ListModelRoutersRequest) (*mgmtpb.ListModelRoutersResponse, error) {
	s.recordCall("ListModelRouters", req)
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	return &mgmtpb.ListModelRoutersResponse{Routers: g.modelRouters, TotalCount: int64(len(g.modelRouters))}, nil
}

// GetModelRouter implements the GetModelRouter RPC.
func (s *TestManagementServer) GetModelRouter(ctx context.Context, req *mgmtpb.GetModelRouterRequest) (*mgmtpb.GetModelRouterResponse, error) {
	s.recordCall("GetModelRouter", req)
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, r := range g.modelRouters {
		if r.Id == req.RouterId {
			return &mgmtpb.GetModelRouterResponse{Router: r}, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "model router %d not found", req.RouterId)
}

// ListSemanticRouters implements the ListSemanticRouters RPC (no paging).
func (s *TestManagementServer) ListSemanticRouters(ctx context.Context, req *mgmtpb.ListSemanticRoutersRequest) (*mgmtpb.ListSemanticRoutersResponse, error) {
	s.recordCall("ListSemanticRouters", req)
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	return &mgmtpb.ListSemanticRoutersResponse{Routers: g.semanticRouters, TotalCount: int64(len(g.semanticRouters))}, nil
}

// GetSemanticRouter implements the GetSemanticRouter RPC.
func (s *TestManagementServer) GetSemanticRouter(ctx context.Context, req *mgmtpb.GetSemanticRouterRequest) (*mgmtpb.GetSemanticRouterResponse, error) {
	s.recordCall("GetSemanticRouter", req)
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, r := range g.semanticRouters {
		if r.Id == req.RouterId {
			return &mgmtpb.GetSemanticRouterResponse{Router: r}, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "semantic router %d not found", req.RouterId)
}

// ListGroups implements the ListGroups RPC.
func (s *TestManagementServer) ListGroups(ctx context.Context, req *mgmtpb.ListGroupsRequest) (*mgmtpb.ListGroupsResponse, error) {
	s.recordCall("ListGroups", req)
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	return &mgmtpb.ListGroupsResponse{Groups: g.groups}, nil
}

// GetResourceInstanceGroups implements the GetResourceInstanceGroups RPC.
func (s *TestManagementServer) GetResourceInstanceGroups(ctx context.Context, req *mgmtpb.GetResourceInstanceGroupsRequest) (*mgmtpb.GetResourceInstanceGroupsResponse, error) {
	s.recordCall("GetResourceInstanceGroups", req)
	if !s.ownsType(req.ResourceTypeSlug) {
		return nil, status.Error(codes.PermissionDenied, "resource type not registered by this plugin")
	}
	return &mgmtpb.GetResourceInstanceGroupsResponse{GroupIds: s.InstanceGroups(req.ResourceTypeSlug, req.InstanceId)}, nil
}

// SetResourceInstanceGroups implements the SetResourceInstanceGroups RPC.
func (s *TestManagementServer) SetResourceInstanceGroups(ctx context.Context, req *mgmtpb.SetResourceInstanceGroupsRequest) (*mgmtpb.SetResourceInstanceGroupsResponse, error) {
	s.recordCall("SetResourceInstanceGroups", req)
	if !s.ownsType(req.ResourceTypeSlug) {
		return nil, status.Error(codes.PermissionDenied, "resource type not registered by this plugin")
	}
	g := s.governance()
	g.mu.Lock()
	defer g.mu.Unlock()
	known := map[uint32]bool{}
	for _, gr := range g.groups {
		known[gr.Id] = true
	}
	for _, id := range req.GroupIds {
		if !known[id] {
			return nil, status.Errorf(codes.InvalidArgument, "unknown group %d", id)
		}
	}
	key := req.ResourceTypeSlug + "|" + req.InstanceId
	set := g.grants[key]
	if set == nil || req.Replace {
		set = map[uint32]bool{}
	}
	for _, id := range req.GroupIds {
		set[id] = true
	}
	g.grants[key] = set
	return &mgmtpb.SetResourceInstanceGroupsResponse{GroupIds: sortedGroupIDs(set)}, nil
}

// ListAccessibleResourceInstances implements the ListAccessibleResourceInstances RPC.
func (s *TestManagementServer) ListAccessibleResourceInstances(ctx context.Context, req *mgmtpb.ListAccessibleResourceInstancesRequest) (*mgmtpb.ListAccessibleResourceInstancesResponse, error) {
	s.recordCall("ListAccessibleResourceInstances", req)
	if !s.ownsType(req.ResourceTypeSlug) {
		return nil, status.Error(codes.PermissionDenied, "resource type not registered by this plugin")
	}
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.userSeesAll[req.UserId] {
		return &mgmtpb.ListAccessibleResourceInstancesResponse{SeeAll: true}, nil
	}
	member := g.userGroups[req.UserId]
	prefix := req.ResourceTypeSlug + "|"
	ids := []string{}
	for key, set := range g.grants {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		for gid := range set {
			if member[gid] {
				ids = append(ids, strings.TrimPrefix(key, prefix))
				break
			}
		}
	}
	sort.Strings(ids)
	return &mgmtpb.ListAccessibleResourceInstancesResponse{InstanceIds: ids}, nil
}

// AddApp adds (or replaces) an App the plugin can read with GetApp/ListApps
// and govern with SetAppGovernanceState.
func (s *TestManagementServer) AddApp(app *mgmtpb.AppInfo) {
	g := s.governance()
	g.mu.Lock()
	g.apps[app.Id] = app
	g.mu.Unlock()
}

// App returns an App as the fake holds it, and its governance flags.
func (s *TestManagementServer) App(id uint32) (*mgmtpb.AppInfo, map[string]string) {
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	flags := map[string]string{}
	for k, v := range g.appFlags[id] {
		flags[k] = v
	}
	return g.apps[id], flags
}

// GetApp implements the GetApp RPC from the Apps added with AddApp.
func (s *TestManagementServer) GetApp(ctx context.Context, req *mgmtpb.GetAppRequest) (*mgmtpb.GetAppResponse, error) {
	s.recordCall("GetApp", req)
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	app, ok := g.apps[req.AppId]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "app not found: %d", req.AppId)
	}
	return &mgmtpb.GetAppResponse{App: app}, nil
}

// ListApps implements the ListApps RPC (no paging or filters).
func (s *TestManagementServer) ListApps(ctx context.Context, req *mgmtpb.ListAppsRequest) (*mgmtpb.ListAppsResponse, error) {
	s.recordCall("ListApps", req)
	g := s.governance()
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]*mgmtpb.AppInfo, 0, len(g.apps))
	for _, a := range g.apps {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return &mgmtpb.ListAppsResponse{Apps: out, TotalCount: int64(len(out))}, nil
}

// SetAppGovernanceState implements the SetAppGovernanceState RPC.
func (s *TestManagementServer) SetAppGovernanceState(ctx context.Context, req *mgmtpb.SetAppGovernanceStateRequest) (*mgmtpb.SetAppGovernanceStateResponse, error) {
	s.recordCall("SetAppGovernanceState", req)
	g := s.governance()
	g.mu.Lock()
	defer g.mu.Unlock()
	app, ok := g.apps[req.AppId]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "app %d not found", req.AppId)
	}
	changed := false
	if req.IsActive != nil && app.IsActive != req.GetIsActive() {
		app.IsActive = req.GetIsActive()
		changed = true
	}
	flags := g.appFlags[req.AppId]
	if flags == nil {
		flags = map[string]string{}
		g.appFlags[req.AppId] = flags
	}
	for k, v := range req.Flags {
		if flags[k] == v {
			continue
		}
		changed = true
		if v == "" {
			delete(flags, k)
		} else {
			flags[k] = v
		}
	}
	return &mgmtpb.SetAppGovernanceStateResponse{App: app, Changed: changed}, nil
}
