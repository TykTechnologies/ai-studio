package grpc

import (
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Trusted prompt templates on edge gateways (features/TrustedPromptTemplates.md).
//
// The snapshot carries the approved templates of the Apps it carries, and
// they count in its checksum: approving, retiring, deleting or editing one
// (topicPromptTemplatesChanged) marks the namespace's edges pending, and an
// edge keeps matching its old set until it applies the next snapshot.
// A template that has lapsed (expires_after_days without a match) is left
// out as it is on the hub. Edges report the templates that matched in their
// heartbeats (recordEdgeTemplateMatches), so a template used only through
// edges does not lapse.

// snapshotPromptTemplates returns the approved, unlapsed templates of apps.
// A template that cannot be read is no template: its App's requests are
// scored whole on the edge, as with none.
func snapshotPromptTemplates(db *gorm.DB, apps []models.App, now time.Time) []*pb.PromptTemplateConfig {
	if len(apps) == 0 {
		return nil
	}
	ids := make([]uint, len(apps))
	for i, a := range apps {
		ids[i] = a.ID
	}
	rows, err := models.ListApprovedPromptTemplatesForApps(db, ids)
	if err != nil {
		logger.Log.Warn().Err(err).Msg("Failed to read prompt templates for the edge snapshot - leaving them out")
		return nil
	}
	var out []*pb.PromptTemplateConfig
	for i := range rows {
		t := &rows[i]
		if t.Lapsed(now) {
			continue
		}
		parts := make([]*pb.PromptTemplatePart, len(t.Parts))
		for k, p := range t.Parts {
			parts[k] = &pb.PromptTemplatePart{Constant: p.Const, Slot: p.Slot}
		}
		out = append(out, &pb.PromptTemplateConfig{
			Id:               uint32(t.ID),
			AppId:            uint32(t.AppID),
			Name:             t.Name,
			Version:          int32(t.Version),
			PlacementRole:    t.PlacementRole,
			PlacementMessage: t.PlacementMessage,
			PlacementAnchor:  t.PlacementAnchor,
			Parts:            parts,
		})
	}
	return out
}

// edgeTemplateTouchEvery bounds how often one template's last_matched_at is
// written for edge matches: expiry counts in days.
const edgeTemplateTouchEvery = 10 * time.Minute

// edgeTemplateTouched is when each template was last touched for an edge
// match on this replica.
var edgeTemplateTouched sync.Map // uint -> time.Time

// recordEdgeTemplateMatches records the templates an edge reported matching
// since its last heartbeat, at most once per edgeTemplateTouchEvery each.
func (s *ControlServer) recordEdgeTemplateMatches(edgeID string, ids []uint32) {
	if len(ids) == 0 {
		return
	}
	now := time.Now()
	var due []uint
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if last, ok := edgeTemplateTouched.Load(uint(id)); ok && now.Sub(last.(time.Time)) < edgeTemplateTouchEvery {
			continue
		}
		edgeTemplateTouched.Store(uint(id), now)
		due = append(due, uint(id))
	}
	if len(due) == 0 {
		return
	}
	if err := models.TouchApprovedAppPromptTemplates(s.db, due, now); err != nil {
		logger.Log.Debug().Err(err).Str("edge_id", edgeID).Msg("Failed to record prompt template matches from an edge")
	}
}
