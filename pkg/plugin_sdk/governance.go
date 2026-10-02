package plugin_sdk

import (
	"context"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MetadataAuditEntry is one change to an object's governed metadata.
type MetadataAuditEntry struct {
	ID            uint32
	Action        string // set | merge | delete
	UserID        uint32
	Source        string // "user", "plugin:<id>", "system", ...
	BeforeJSON    string
	AfterJSON     string
	HooksExecuted []string
	CreatedAt     time.Time
}

// AuditRecord is one platform audit trail record, without request or
// response bodies.
type AuditRecord struct {
	ID           uint32
	Timestamp    time.Time
	UserID       uint32
	UserEmail    string
	UserName     string
	Action       string
	Method       string
	Route        string
	Status       int
	ResourceType string
	ResourceID   string
	ResourceName string
	DiffJSON     string // {"field": {"old":..,"new":..}}, sensitive columns redacted
	Error        string
}

// MCPServerSummary is an MCP server as governance plugins see it.
type MCPServerSummary struct {
	ID             uint32
	Name           string
	Slug           string
	Description    string
	Kind           string
	PrivacyScore   *int // nil until an administrator sets it
	IsActive       bool
	DashboardState string
	Brokerable     bool
	Tags           []string
	UpdatedAt      time.Time
}

// RouterSummary is a model or semantic router and where it can send requests.
type RouterSummary struct {
	ID             uint32
	Kind           string // "model_router" | "semantic_router"
	Name           string
	Slug           string
	Namespace      string
	Description    string
	Active         bool
	LLMIDs         []uint32
	ModelRouterIDs []uint32 // semantic routers only: targets that are model routers
	UpdatedAt      time.Time
}

// GroupSummary is a team.
type GroupSummary struct {
	ID        uint32
	Name      string
	IsDefault bool
}

func tsTime(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.AsTime()
}

func (s *studioServicesImpl) ListObjectMetadataAudit(ctx context.Context, objectType, objectID string, limit int) ([]MetadataAuditEntry, error) {
	resp, err := ai_studio_sdk.ListObjectMetadataAudit(ctx, objectType, objectID, int32(limit))
	if err != nil {
		return nil, err
	}
	out := make([]MetadataAuditEntry, 0, len(resp.GetEntries()))
	for _, e := range resp.GetEntries() {
		out = append(out, MetadataAuditEntry{
			ID:            e.GetId(),
			Action:        e.GetAction(),
			UserID:        e.GetUserId(),
			Source:        e.GetSource(),
			BeforeJSON:    e.GetBeforeJson(),
			AfterJSON:     e.GetAfterJson(),
			HooksExecuted: e.GetHooksExecuted(),
			CreatedAt:     tsTime(e.GetCreatedAt()),
		})
	}
	return out, nil
}

func (s *studioServicesImpl) ListAuditRecords(ctx context.Context, resourceType, resourceID string, limit int, mutationsOnly bool) ([]AuditRecord, error) {
	resp, err := ai_studio_sdk.ListAuditRecords(ctx, resourceType, resourceID, int32(limit), mutationsOnly)
	if err != nil {
		return nil, err
	}
	out := make([]AuditRecord, 0, len(resp.GetRecords()))
	for _, r := range resp.GetRecords() {
		out = append(out, AuditRecord{
			ID:           r.GetId(),
			Timestamp:    tsTime(r.GetTimestamp()),
			UserID:       r.GetUserId(),
			UserEmail:    r.GetUserEmail(),
			UserName:     r.GetUserName(),
			Action:       r.GetAction(),
			Method:       r.GetMethod(),
			Route:        r.GetRoute(),
			Status:       int(r.GetStatus()),
			ResourceType: r.GetResourceType(),
			ResourceID:   r.GetResourceId(),
			ResourceName: r.GetResourceName(),
			DiffJSON:     r.GetDiffJson(),
			Error:        r.GetError(),
		})
	}
	return out, nil
}

func mcpSummary(m *mgmtpb.MCPServerInfo) MCPServerSummary {
	out := MCPServerSummary{
		ID:             m.GetId(),
		Name:           m.GetName(),
		Slug:           m.GetSlug(),
		Description:    m.GetDescription(),
		Kind:           m.GetKind(),
		IsActive:       m.GetIsActive(),
		DashboardState: m.GetDashboardState(),
		Brokerable:     m.GetBrokerable(),
		Tags:           m.GetTags(),
		UpdatedAt:      tsTime(m.GetUpdatedAt()),
	}
	if m.PrivacyScore != nil {
		v := int(m.GetPrivacyScore())
		out.PrivacyScore = &v
	}
	return out
}

func (s *studioServicesImpl) ListMCPServers(ctx context.Context, page, limit int32) ([]MCPServerSummary, int64, error) {
	resp, err := ai_studio_sdk.ListMCPServers(ctx, page, limit, nil)
	if err != nil {
		return nil, 0, err
	}
	out := make([]MCPServerSummary, 0, len(resp.GetServers()))
	for _, m := range resp.GetServers() {
		out = append(out, mcpSummary(m))
	}
	return out, resp.GetTotalCount(), nil
}

func (s *studioServicesImpl) GetMCPServer(ctx context.Context, serverID uint32) (*MCPServerSummary, error) {
	resp, err := ai_studio_sdk.GetMCPServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	out := mcpSummary(resp.GetServer())
	return &out, nil
}

func modelRouterSummary(r *mgmtpb.ModelRouterInfo) RouterSummary {
	return RouterSummary{
		ID:          r.GetId(),
		Kind:        "model_router",
		Name:        r.GetName(),
		Slug:        r.GetSlug(),
		Namespace:   r.GetNamespace(),
		Description: r.GetDescription(),
		Active:      r.GetActive(),
		LLMIDs:      r.GetLlmIds(),
		UpdatedAt:   tsTime(r.GetUpdatedAt()),
	}
}

func semanticRouterSummary(r *mgmtpb.SemanticRouterInfo) RouterSummary {
	return RouterSummary{
		ID:             r.GetId(),
		Kind:           "semantic_router",
		Name:           r.GetName(),
		Slug:           r.GetSlug(),
		Namespace:      r.GetNamespace(),
		Description:    r.GetDescription(),
		Active:         r.GetActive(),
		LLMIDs:         r.GetLlmIds(),
		ModelRouterIDs: r.GetModelRouterIds(),
		UpdatedAt:      tsTime(r.GetUpdatedAt()),
	}
}

func (s *studioServicesImpl) ListModelRouters(ctx context.Context, page, limit int32) ([]RouterSummary, int64, error) {
	resp, err := ai_studio_sdk.ListModelRouters(ctx, page, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]RouterSummary, 0, len(resp.GetRouters()))
	for _, r := range resp.GetRouters() {
		out = append(out, modelRouterSummary(r))
	}
	return out, resp.GetTotalCount(), nil
}

func (s *studioServicesImpl) GetModelRouter(ctx context.Context, routerID uint32) (*RouterSummary, error) {
	resp, err := ai_studio_sdk.GetModelRouter(ctx, routerID)
	if err != nil {
		return nil, err
	}
	out := modelRouterSummary(resp.GetRouter())
	return &out, nil
}

func (s *studioServicesImpl) ListSemanticRouters(ctx context.Context, page, limit int32) ([]RouterSummary, int64, error) {
	resp, err := ai_studio_sdk.ListSemanticRouters(ctx, page, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]RouterSummary, 0, len(resp.GetRouters()))
	for _, r := range resp.GetRouters() {
		out = append(out, semanticRouterSummary(r))
	}
	return out, resp.GetTotalCount(), nil
}

func (s *studioServicesImpl) GetSemanticRouter(ctx context.Context, routerID uint32) (*RouterSummary, error) {
	resp, err := ai_studio_sdk.GetSemanticRouter(ctx, routerID)
	if err != nil {
		return nil, err
	}
	out := semanticRouterSummary(resp.GetRouter())
	return &out, nil
}

func (s *studioServicesImpl) ListGroups(ctx context.Context) ([]GroupSummary, error) {
	resp, err := ai_studio_sdk.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]GroupSummary, 0, len(resp.GetGroups()))
	for _, g := range resp.GetGroups() {
		out = append(out, GroupSummary{ID: g.GetId(), Name: g.GetName(), IsDefault: g.GetIsDefault()})
	}
	return out, nil
}

func (s *studioServicesImpl) GetResourceInstanceGroups(ctx context.Context, resourceTypeSlug, instanceID string) ([]uint32, error) {
	resp, err := ai_studio_sdk.GetResourceInstanceGroups(ctx, resourceTypeSlug, instanceID)
	if err != nil {
		return nil, err
	}
	return resp.GetGroupIds(), nil
}

func (s *studioServicesImpl) SetResourceInstanceGroups(ctx context.Context, resourceTypeSlug, instanceID string, groupIDs []uint32, replace bool) ([]uint32, error) {
	resp, err := ai_studio_sdk.SetResourceInstanceGroups(ctx, resourceTypeSlug, instanceID, groupIDs, replace)
	if err != nil {
		return nil, err
	}
	return resp.GetGroupIds(), nil
}

func (s *studioServicesImpl) ListAccessibleResourceInstances(ctx context.Context, resourceTypeSlug string, userID uint32) (bool, []string, error) {
	resp, err := ai_studio_sdk.ListAccessibleResourceInstances(ctx, resourceTypeSlug, userID)
	if err != nil {
		return false, nil, err
	}
	return resp.GetSeeAll(), resp.GetInstanceIds(), nil
}

func (s *studioServicesImpl) SetAppGovernanceState(ctx context.Context, appID uint32, isActive *bool, flags map[string]string, reason string) (bool, error) {
	resp, err := ai_studio_sdk.SetAppGovernanceState(ctx, appID, isActive, flags, reason)
	if err != nil {
		return false, err
	}
	return resp.GetChanged(), nil
}
