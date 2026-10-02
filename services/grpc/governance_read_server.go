package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/audit"
	"github.com/TykTechnologies/midsommar/v2/services/model_router"
	"github.com/TykTechnologies/midsommar/v2/services/semantic_router"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Read-only management RPCs for governance plugins (an asset catalog
// building a dependency graph across core objects), and the team-grant RPCs
// a ResourceProvider plugin uses to decide who sees its own instances.
//
// Every method checks its scope with validatePluginScope, which loads the
// plugin, so brokered sessions (plugin ID only in the context) work too.

const (
	defaultGovernanceListLimit = 50
	maxGovernanceListLimit     = 200
)

func clampLimit(limit int32, def, max int) int {
	if limit <= 0 {
		return def
	}
	if int(limit) > max {
		return max
	}
	return int(limit)
}

func jsonString(v interface{}) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return ""
	}
	return string(b)
}

// --- Governed metadata history ---

// ListObjectMetadataAudit returns the governed metadata audit trail of one
// object, newest first. Community Edition returns an empty list.
func (s *AIStudioManagementServer) ListObjectMetadataAudit(ctx context.Context, req *pb.ListObjectMetadataAuditRequest) (*pb.ListObjectMetadataAuditResponse, error) {
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeMetadataRead)
	if err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	objectType, err := resolveObjectType(SetPluginInContext(ctx, plugin), req.GetObjectType())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.GetObjectId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "object_id is required")
	}
	rows, err := s.service.GovernedMetadata().ListAudit(objectType, req.GetObjectId(), clampLimit(req.GetLimit(), 100, 500))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list metadata audit: %v", err)
	}
	out := make([]*pb.ObjectMetadataAuditEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, &pb.ObjectMetadataAuditEntry{
			Id:            uint32(r.ID),
			Action:        r.Action,
			UserId:        uint32(r.UserID),
			Source:        r.Source,
			BeforeJson:    jsonString(r.Before),
			AfterJson:     jsonString(r.After),
			HooksExecuted: r.HookExecuted,
			CreatedAt:     timestamppb.New(r.CreatedAt),
		})
	}
	return &pb.ListObjectMetadataAuditResponse{Entries: out}, nil
}

// --- Platform audit trail ---

// auditMutationMethods are the audit record methods that change something:
// the HTTP writes, plus "RPC" for changes plugins make through management
// RPCs (SetAppGovernanceState).
var auditMutationMethods = []string{"POST", "PUT", "PATCH", "DELETE", "RPC"}

// ListAuditRecords returns the platform audit records for one resource,
// newest first, without request or response bodies. Community Edition
// answers Unimplemented; a deployment that does not store records in the
// database answers FailedPrecondition.
func (s *AIStudioManagementServer) ListAuditRecords(ctx context.Context, req *pb.ListAuditRecordsRequest) (*pb.ListAuditRecordsResponse, error) {
	if _, err := s.validatePluginScope(ctx, models.ServiceScopeAuditRead); err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	resourceType := strings.TrimSpace(req.GetResourceType())
	resourceID := strings.TrimSpace(req.GetResourceId())
	if resourceType == "" || resourceID == "" {
		return nil, status.Error(codes.InvalidArgument, "resource_type and resource_id are required")
	}
	limit := clampLimit(req.GetLimit(), defaultGovernanceListLimit, maxGovernanceListLimit)
	auditSvc := s.service.Audit()
	if auditSvc == nil {
		// Not attached (proxy-only mode, or before the API is built).
		return nil, status.Error(codes.FailedPrecondition, "the audit trail is not available on this node")
	}

	methods := []string{""}
	if req.GetMutationsOnly() {
		methods = auditMutationMethods
	}
	var records []models.AuditRecord
	for _, m := range methods {
		page, err := auditSvc.List(ctx, audit.Query{
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Method:       m,
			Page:         1,
			PageSize:     limit,
		})
		if err != nil {
			switch {
			case errors.Is(err, audit.ErrEnterpriseFeature):
				return nil, status.Error(codes.Unimplemented, "the audit trail is an Enterprise Edition feature")
			case errors.Is(err, audit.ErrDisabled):
				return nil, status.Error(codes.FailedPrecondition, err.Error())
			}
			return nil, status.Errorf(codes.Internal, "failed to list audit records: %v", err)
		}
		records = append(records, page.Records...)
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Timestamp.After(records[j].Timestamp) })
	if len(records) > limit {
		records = records[:limit]
	}
	out := make([]*pb.AuditRecordInfo, 0, len(records))
	for _, r := range records {
		out = append(out, &pb.AuditRecordInfo{
			Id:           uint32(r.ID),
			Timestamp:    timestamppb.New(r.Timestamp),
			UserId:       uint32(r.UserID),
			UserEmail:    r.UserEmail,
			UserName:     r.UserName,
			Action:       r.Action,
			Method:       r.Method,
			Route:        r.Route,
			Status:       int32(r.Status),
			ResourceType: r.ResourceType,
			ResourceId:   r.ResourceID,
			ResourceName: r.ResourceName,
			// Diff is redacted before storage; dumps are never returned.
			DiffJson: string(r.Diff),
			Error:    r.Error,
		})
	}
	return &pb.ListAuditRecordsResponse{Records: out}, nil
}

// --- MCP servers ---

func mcpServerToPB(m *models.MCPServer) *pb.MCPServerInfo {
	info := &pb.MCPServerInfo{
		Id:             uint32(m.ID),
		Name:           m.Name,
		Slug:           m.Slug,
		Description:    m.Description,
		Kind:           m.Kind,
		IsActive:       m.IsActive,
		DashboardState: m.DashboardState,
		Brokerable:     m.Brokerable,
		Tags:           m.Tags(),
		UpdatedAt:      timestamppb.New(m.UpdatedAt),
	}
	if m.PrivacyScore != nil {
		v := int32(*m.PrivacyScore)
		info.PrivacyScore = &v
	}
	return info
}

// ListMCPServers lists MCP servers without any upstream or auth detail.
func (s *AIStudioManagementServer) ListMCPServers(ctx context.Context, req *pb.ListMCPServersRequest) (*pb.ListMCPServersResponse, error) {
	if _, err := s.validatePluginScope(ctx, models.ServiceScopeMCPServersRead); err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	limit := clampLimit(req.GetLimit(), defaultGovernanceListLimit, maxGovernanceListLimit)
	page := int(req.GetPage())
	if page < 1 {
		page = 1
	}
	q := s.service.GetDB().Model(&models.MCPServer{})
	if req.IsActive != nil {
		q = q.Where("is_active = ?", req.GetIsActive())
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count MCP servers: %v", err)
	}
	var rows []models.MCPServer
	if err := q.Order("name").Order("id").Limit(limit).Offset((page - 1) * limit).Find(&rows).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list MCP servers: %v", err)
	}
	out := make([]*pb.MCPServerInfo, 0, len(rows))
	for i := range rows {
		out = append(out, mcpServerToPB(&rows[i]))
	}
	return &pb.ListMCPServersResponse{Servers: out, TotalCount: total}, nil
}

// GetMCPServer returns one MCP server without any upstream or auth detail.
func (s *AIStudioManagementServer) GetMCPServer(ctx context.Context, req *pb.GetMCPServerRequest) (*pb.GetMCPServerResponse, error) {
	if _, err := s.validatePluginScope(ctx, models.ServiceScopeMCPServersRead); err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	var m models.MCPServer
	if err := s.service.GetDB().First(&m, req.GetServerId()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "MCP server %d not found", req.GetServerId())
		}
		return nil, status.Errorf(codes.Internal, "failed to get MCP server: %v", err)
	}
	return &pb.GetMCPServerResponse{Server: mcpServerToPB(&m)}, nil
}

// --- Routers ---

// modelRouterLLMIDs maps each router to the LLMs its pool vendors name.
func modelRouterLLMIDs(db *gorm.DB, routerIDs []uint) (map[uint][]uint32, error) {
	out := make(map[uint][]uint32, len(routerIDs))
	if len(routerIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		RouterID uint
		LLMID    uint
	}
	err := db.Table("pool_vendors").
		Select("DISTINCT model_pools.router_id AS router_id, pool_vendors.llm_id AS llm_id").
		Joins("JOIN model_pools ON model_pools.id = pool_vendors.pool_id AND model_pools.deleted_at IS NULL").
		Where("model_pools.router_id IN ? AND pool_vendors.deleted_at IS NULL", routerIDs).
		Order("model_pools.router_id, pool_vendors.llm_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.RouterID] = append(out[r.RouterID], uint32(r.LLMID))
	}
	return out, nil
}

func modelRouterToPB(r *models.ModelRouter, llmIDs []uint32) *pb.ModelRouterInfo {
	return &pb.ModelRouterInfo{
		Id:          uint32(r.ID),
		Name:        r.Name,
		Slug:        r.Slug,
		Namespace:   r.Namespace,
		Description: r.Description,
		Active:      r.Active,
		LlmIds:      llmIDs,
		UpdatedAt:   timestamppb.New(r.UpdatedAt),
	}
}

// semanticRouterTargets collects the LLMs (route targets and the judge model)
// and model routers (route targets) a semantic router can send requests to.
func semanticRouterTargets(r *models.SemanticRouter) (llmIDs, routerIDs []uint32) {
	llms := map[uint32]bool{}
	routers := map[uint32]bool{}
	for _, route := range r.Routes {
		if route.Target.LLMID != 0 {
			llms[uint32(route.Target.LLMID)] = true
		}
		if route.Target.ModelRouterID != 0 {
			routers[uint32(route.Target.ModelRouterID)] = true
		}
	}
	if r.Settings.Judge.Enabled && r.Settings.Judge.ModelRef.LLMID != 0 {
		llms[uint32(r.Settings.Judge.ModelRef.LLMID)] = true
	}
	return sortedIDs(llms), sortedIDs(routers)
}

func sortedIDs(set map[uint32]bool) []uint32 {
	out := make([]uint32, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func semanticRouterToPB(r *models.SemanticRouter) *pb.SemanticRouterInfo {
	llms, routers := semanticRouterTargets(r)
	return &pb.SemanticRouterInfo{
		Id:             uint32(r.ID),
		Name:           r.Name,
		Slug:           r.Slug,
		Namespace:      r.Namespace,
		Description:    r.Description,
		Active:         r.Active,
		LlmIds:         llms,
		ModelRouterIds: routers,
		UpdatedAt:      timestamppb.New(r.UpdatedAt),
	}
}

func routerError(err error, what string) error {
	if errors.Is(err, model_router.ErrEnterpriseFeature) || errors.Is(err, semantic_router.ErrEnterpriseFeature) {
		return status.Errorf(codes.Unimplemented, "%s are an Enterprise Edition feature", what)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return status.Errorf(codes.NotFound, "%s not found", what)
	}
	return status.Errorf(codes.Internal, "failed to load %s: %v", what, err)
}

func pageOf(page, limit int32) (int, int) {
	l := clampLimit(limit, defaultGovernanceListLimit, maxGovernanceListLimit)
	p := int(page)
	if p < 1 {
		p = 1
	}
	return p, l
}

// ListModelRouters lists model routers with the LLMs each can route to.
func (s *AIStudioManagementServer) ListModelRouters(ctx context.Context, req *pb.ListModelRoutersRequest) (*pb.ListModelRoutersResponse, error) {
	if _, err := s.validatePluginScope(ctx, models.ServiceScopeRoutersRead); err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	page, limit := pageOf(req.GetPage(), req.GetLimit())
	routers, total, _, err := s.service.ListModelRouters(limit, page, false)
	if err != nil {
		return nil, routerError(err, "model routers")
	}
	ids := make([]uint, 0, len(routers))
	for _, r := range routers {
		ids = append(ids, r.ID)
	}
	llms, err := modelRouterLLMIDs(s.service.GetDB(), ids)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to resolve router targets: %v", err)
	}
	out := make([]*pb.ModelRouterInfo, 0, len(routers))
	for i := range routers {
		out = append(out, modelRouterToPB(&routers[i], llms[routers[i].ID]))
	}
	return &pb.ListModelRoutersResponse{Routers: out, TotalCount: total}, nil
}

// GetModelRouter returns one model router with the LLMs it can route to.
func (s *AIStudioManagementServer) GetModelRouter(ctx context.Context, req *pb.GetModelRouterRequest) (*pb.GetModelRouterResponse, error) {
	if _, err := s.validatePluginScope(ctx, models.ServiceScopeRoutersRead); err != nil {
		return nil, err
	}
	if s.service == nil || s.service.ModelRouterService == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	r, err := s.service.ModelRouterService.GetRouter(uint(req.GetRouterId()))
	if err != nil {
		return nil, routerError(err, "model router")
	}
	llms, err := modelRouterLLMIDs(s.service.GetDB(), []uint{r.ID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to resolve router targets: %v", err)
	}
	return &pb.GetModelRouterResponse{Router: modelRouterToPB(r, llms[r.ID])}, nil
}

// ListSemanticRouters lists semantic routers with their targets.
func (s *AIStudioManagementServer) ListSemanticRouters(ctx context.Context, req *pb.ListSemanticRoutersRequest) (*pb.ListSemanticRoutersResponse, error) {
	if _, err := s.validatePluginScope(ctx, models.ServiceScopeRoutersRead); err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	page, limit := pageOf(req.GetPage(), req.GetLimit())
	routers, total, _, err := s.service.ListSemanticRouters(limit, page, false)
	if err != nil {
		return nil, routerError(err, "semantic routers")
	}
	out := make([]*pb.SemanticRouterInfo, 0, len(routers))
	for i := range routers {
		out = append(out, semanticRouterToPB(&routers[i]))
	}
	return &pb.ListSemanticRoutersResponse{Routers: out, TotalCount: total}, nil
}

// GetSemanticRouter returns one semantic router with its targets.
func (s *AIStudioManagementServer) GetSemanticRouter(ctx context.Context, req *pb.GetSemanticRouterRequest) (*pb.GetSemanticRouterResponse, error) {
	if _, err := s.validatePluginScope(ctx, models.ServiceScopeRoutersRead); err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	svc := s.service.SemanticRouterService
	if svc == nil {
		svc = semantic_router.NewService(s.service.GetDB())
	}
	r, err := svc.GetRouter(uint(req.GetRouterId()))
	if err != nil {
		return nil, routerError(err, "semantic router")
	}
	return &pb.GetSemanticRouterResponse{Router: semanticRouterToPB(r)}, nil
}

// --- Team access to the plugin's own resource instances ---

func resourceAccessError(err error) error {
	switch {
	case errors.Is(err, services.ErrResourceTypeNotOwned):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, services.ErrUnknownGroup):
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return status.Errorf(codes.Internal, "%v", err)
}

func uintsToPB(ids []uint) []uint32 {
	out := make([]uint32, len(ids))
	for i, id := range ids {
		out[i] = uint32(id)
	}
	return out
}

// ListGroups lists every team, with the Default team flagged.
func (s *AIStudioManagementServer) ListGroups(ctx context.Context, req *pb.ListGroupsRequest) (*pb.ListGroupsResponse, error) {
	if _, err := s.validatePluginScope(ctx, models.ServiceScopeResourceAccessManage); err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	groups, err := s.service.ListGroupsForPlugin()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list groups: %v", err)
	}
	out := make([]*pb.GroupInfo, 0, len(groups))
	for _, g := range groups {
		out = append(out, &pb.GroupInfo{Id: uint32(g.ID), Name: g.Name, IsDefault: g.IsDefault})
	}
	return &pb.ListGroupsResponse{Groups: out}, nil
}

// GetResourceInstanceGroups returns the teams granted one of the calling
// plugin's resource instances.
func (s *AIStudioManagementServer) GetResourceInstanceGroups(ctx context.Context, req *pb.GetResourceInstanceGroupsRequest) (*pb.GetResourceInstanceGroupsResponse, error) {
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeResourceAccessManage)
	if err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	ids, err := s.service.ResourceInstanceGroups(plugin.ID, req.GetResourceTypeSlug(), req.GetInstanceId())
	if err != nil {
		return nil, resourceAccessError(err)
	}
	return &pb.GetResourceInstanceGroupsResponse{GroupIds: uintsToPB(ids)}, nil
}

// SetResourceInstanceGroups adds or replaces the teams granted one of the
// calling plugin's resource instances.
func (s *AIStudioManagementServer) SetResourceInstanceGroups(ctx context.Context, req *pb.SetResourceInstanceGroupsRequest) (*pb.SetResourceInstanceGroupsResponse, error) {
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeResourceAccessManage)
	if err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	if strings.TrimSpace(req.GetInstanceId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "instance_id is required")
	}
	groupIDs := make([]uint, len(req.GetGroupIds()))
	for i, id := range req.GetGroupIds() {
		groupIDs[i] = uint(id)
	}
	ids, err := s.service.SetResourceInstanceGroups(plugin.ID, req.GetResourceTypeSlug(), req.GetInstanceId(), groupIDs, req.GetReplace())
	if err != nil {
		return nil, resourceAccessError(err)
	}
	return &pb.SetResourceInstanceGroupsResponse{GroupIds: uintsToPB(ids)}, nil
}

// ListAccessibleResourceInstances returns which of the calling plugin's
// instances of a type a user can reach, under the platform's portal rule.
func (s *AIStudioManagementServer) ListAccessibleResourceInstances(ctx context.Context, req *pb.ListAccessibleResourceInstancesRequest) (*pb.ListAccessibleResourceInstancesResponse, error) {
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeResourceAccessManage)
	if err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	if req.GetUserId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	seeAll, ids, err := s.service.AccessibleResourceInstanceIDs(ctx, plugin.ID, req.GetResourceTypeSlug(), uint(req.GetUserId()))
	if err != nil {
		return nil, resourceAccessError(err)
	}
	return &pb.ListAccessibleResourceInstancesResponse{SeeAll: seeAll, InstanceIds: ids}, nil
}

// --- App bindings ---

// fillAppBindings adds the MCP server, router and plugin resource bindings
// to app infos (convertAppToPB fills LLMs, tools and data sources from the
// preloaded associations). One query per join table for the whole batch.
func fillAppBindings(db *gorm.DB, apps []*pb.AppInfo) error {
	if db == nil || len(apps) == 0 {
		return nil
	}
	byID := make(map[uint32]*pb.AppInfo, len(apps))
	ids := make([]uint32, 0, len(apps))
	for _, a := range apps {
		if a == nil {
			continue
		}
		byID[a.Id] = a
		ids = append(ids, a.Id)
	}
	if len(ids) == 0 {
		return nil
	}

	joins := []struct {
		table, column string
		set           func(*pb.AppInfo, uint32)
	}{
		{"app_mcp_servers", "mcp_server_id", func(a *pb.AppInfo, id uint32) { a.McpServerIds = append(a.McpServerIds, id) }},
		{"app_model_routers", "model_router_id", func(a *pb.AppInfo, id uint32) { a.ModelRouterIds = append(a.ModelRouterIds, id) }},
		{"app_semantic_routers", "semantic_router_id", func(a *pb.AppInfo, id uint32) { a.SemanticRouterIds = append(a.SemanticRouterIds, id) }},
	}
	for _, j := range joins {
		var rows []struct {
			AppID uint32
			RefID uint32
		}
		if err := db.Table(j.table).
			Select("app_id AS app_id, "+j.column+" AS ref_id").
			Where("app_id IN ?", ids).
			Order("app_id, " + j.column).
			Scan(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			if a := byID[r.AppID]; a != nil {
				j.set(a, r.RefID)
			}
		}
	}

	var prs []models.AppPluginResource
	if err := db.Where("app_id IN ?", ids).Preload("PluginResourceType").Order("app_id, id").Find(&prs).Error; err != nil {
		return err
	}
	for _, pr := range prs {
		a := byID[uint32(pr.AppID)]
		if a == nil || pr.PluginResourceType == nil {
			continue
		}
		a.PluginResources = append(a.PluginResources, &pb.AppPluginResourceRef{
			PluginId:         uint32(pr.PluginResourceType.PluginID),
			ResourceTypeSlug: pr.PluginResourceType.Slug,
			InstanceId:       pr.InstanceID,
		})
	}
	return nil
}
