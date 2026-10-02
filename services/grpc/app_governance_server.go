package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// maxAppGovernanceFlags bounds one call; flag names are short identifiers.
const maxAppGovernanceFlags = 16

// SetAppGovernanceState lets a governance plugin suspend or reactivate an
// App and raise or clear governance flags on it. It cannot change what the
// App may access. Every change is written to the audit trail, attributed to
// the plugin, with the reason. Requires apps.lifecycle.
func (s *AIStudioManagementServer) SetAppGovernanceState(ctx context.Context, req *pb.SetAppGovernanceStateRequest) (*pb.SetAppGovernanceStateResponse, error) {
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeAppsLifecycle)
	if err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	if req.GetAppId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "app_id is required")
	}
	if len(req.GetFlags()) > maxAppGovernanceFlags {
		return nil, status.Errorf(codes.InvalidArgument, "at most %d flags per call", maxAppGovernanceFlags)
	}
	for name, value := range req.GetFlags() {
		if name == "" || len(name) > 64 || strings.ContainsAny(name, " \t\n") || len(value) > 256 {
			return nil, status.Errorf(codes.InvalidArgument, "invalid flag %q", name)
		}
	}
	reason := strings.TrimSpace(req.GetReason())
	if len(reason) > 1024 {
		reason = reason[:1024]
	}
	setBy := fmt.Sprintf("plugin:%d", plugin.ID)
	var active *bool
	if req.IsActive != nil {
		v := req.GetIsActive()
		active = &v
	}
	app, diff, err := s.service.SetAppGovernanceState(uint(req.GetAppId()), active, req.GetFlags(), reason, setBy)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "app %d not found", req.GetAppId())
		}
		return nil, status.Errorf(codes.Internal, "failed to update app: %v", err)
	}
	if len(diff) > 0 {
		s.recordPluginAppChange(ctx, plugin, app, diff, reason)
	}
	info := convertAppToPB(app)
	if err := fillAppBindings(s.service.GetDB(), []*pb.AppInfo{info}); err != nil {
		log.Warn().Err(err).Uint32("app_id", req.GetAppId()).Msg("Failed to load app bindings")
	}
	return &pb.SetAppGovernanceStateResponse{App: info, Changed: len(diff) > 0}, nil
}

// recordPluginAppChange writes a non-HTTP audit record for a plugin's change.
func (s *AIStudioManagementServer) recordPluginAppChange(ctx context.Context, plugin *models.Plugin, app *models.App, diff map[string]interface{}, reason string) {
	auditSvc := s.service.Audit()
	if auditSvc == nil {
		return
	}
	if reason != "" {
		diff["reason"] = map[string]interface{}{"old": nil, "new": reason}
	}
	raw, _ := json.Marshal(diff)
	rec := &models.AuditRecord{
		Timestamp:    time.Now().UTC(),
		UserName:     "plugin: " + plugin.Name,
		Action:       "Plugin Update App Governance State",
		Method:       "RPC",
		Route:        "SetAppGovernanceState",
		Status:       200,
		ResourceType: "app",
		ResourceID:   fmt.Sprint(app.ID),
		ResourceName: app.Name,
		Diff:         models.RawJSON(raw),
	}
	if err := auditSvc.Record(ctx, rec); err != nil {
		log.Warn().Err(err).Uint("app_id", app.ID).Msg("Failed to record plugin app change in the audit trail")
	}
}
