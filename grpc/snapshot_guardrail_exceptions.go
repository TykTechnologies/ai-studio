package grpc

import (
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Guardrail exceptions on edge gateways (features/GuardrailExceptions.md).
//
// The snapshot carries the approved exceptions of the Apps it carries, each
// with the capability of its App, and they count in its checksum: approving,
// retiring, deleting or changing one, or attesting its App's capability
// (topicGuardrailExceptionsChanged, topicAppUpdated), marks the namespace's
// edges pending, and an edge keeps applying its old set until it applies the
// next snapshot. An exception that has lapsed (expires_after_days without a
// hit) or that is outside the floor for its App's capability is left out.
// Edges report the exceptions that applied in their heartbeats
// (recordEdgeGuardrailExceptionHits), so an exception used only through
// edges does not lapse.

// snapshotGuardrailExceptions returns the approved, unlapsed exceptions of
// apps that are within their App's floor. An exception that cannot be read
// is no exception: the filters' actions apply on the edge, as with none.
func snapshotGuardrailExceptions(db *gorm.DB, apps []models.App, now time.Time) []*pb.GuardrailExceptionConfig {
	if len(apps) == 0 {
		return nil
	}
	ids := make([]uint, len(apps))
	capability := make(map[uint]guardrails.Capability, len(apps))
	for i, a := range apps {
		ids[i] = a.ID
		capability[a.ID] = guardrails.Capability(a.GuardrailCapability)
	}
	rows, err := models.ListApprovedGuardrailExceptionsForApps(db, ids)
	if err != nil {
		logger.Log.Warn().Err(err).Msg("Failed to read guardrail exceptions for the edge snapshot - leaving them out")
		return nil
	}
	var out []*pb.GuardrailExceptionConfig
	for i := range rows {
		e := &rows[i]
		c := capability[e.AppID]
		if e.Lapsed(now) || !guardrails.Allowed(c, guardrails.Channel(e.Channel), e.Action) {
			continue
		}
		out = append(out, &pb.GuardrailExceptionConfig{
			Id:            uint32(e.ID),
			AppId:         uint32(e.AppID),
			FilterId:      uint32(e.FilterID),
			Detector:      e.Detector,
			Channel:       e.Channel,
			ToolName:      e.ToolName,
			Action:        e.Action,
			AppCapability: string(c),
		})
	}
	return out
}

// edgeExceptionHitFlushEvery bounds how often one exception's edge hits are
// written on this replica.
const edgeExceptionHitFlushEvery = time.Minute

// edgeExceptionHitWritten is when each exception's edge hits were last
// written on this replica.
var edgeExceptionHitWritten sync.Map // uint -> time.Time

// recordEdgeGuardrailExceptionHits records the exceptions an edge reported
// applying since its last heartbeat. Only exceptions the edge's own
// snapshot carries count: approved, unlapsed, within the floor, of an
// active App and a filter in the edge's namespace. Anything else an edge
// reports (a compromised edge keeping another namespace's exception from
// lapsing) is dropped. Each counts as one hit per heartbeat, written at most
// once per edgeExceptionHitFlushEvery.
func (s *ControlServer) recordEdgeGuardrailExceptionHits(edgeID, namespace string, ids []uint32) {
	if len(ids) == 0 {
		return
	}
	want := make([]uint, 0, len(ids))
	seen := map[uint]bool{}
	for _, id := range ids {
		if id != 0 && !seen[uint(id)] {
			seen[uint(id)] = true
			want = append(want, uint(id))
		}
	}
	if len(want) == 0 {
		return
	}
	namespace = models.CanonicalNamespace(namespace)
	var rows []struct {
		models.GuardrailException
		Capability string
	}
	q := s.db.Table("guardrail_exceptions").
		Select("guardrail_exceptions.*, apps.guardrail_capability AS capability").
		Joins("JOIN apps ON apps.id = guardrail_exceptions.app_id AND apps.deleted_at IS NULL AND apps.is_active = ?", true).
		Joins("JOIN filters ON filters.id = guardrail_exceptions.filter_id AND filters.deleted_at IS NULL").
		Where("guardrail_exceptions.id IN ? AND guardrail_exceptions.status = ?", want, models.GuardrailExceptionApproved)
	if namespace == "" {
		q = q.Where("apps.namespace = '' AND filters.namespace = ''")
	} else {
		q = q.Where("(apps.namespace = '' OR apps.namespace = ?) AND (filters.namespace = '' OR filters.namespace = ?)", namespace, namespace)
	}
	if err := q.Find(&rows).Error; err != nil {
		logger.Log.Debug().Err(err).Str("edge_id", edgeID).Msg("Failed to read guardrail exceptions an edge reported")
		return
	}
	now := time.Now()
	accepted := 0
	for i := range rows {
		e := &rows[i].GuardrailException
		if e.Lapsed(now) || !guardrails.Allowed(guardrails.Capability(rows[i].Capability), guardrails.Channel(e.Channel), e.Action) {
			continue
		}
		accepted++
		if last, ok := edgeExceptionHitWritten.Load(e.ID); ok && now.Sub(last.(time.Time)) < edgeExceptionHitFlushEvery {
			continue
		}
		edgeExceptionHitWritten.Store(e.ID, now)
		if err := models.AddGuardrailExceptionHits(s.db, e.ID, 1, now); err != nil {
			logger.Log.Debug().Err(err).Str("edge_id", edgeID).Msg("Failed to record a guardrail exception hit from an edge")
		}
	}
	if dropped := len(want) - accepted; dropped > 0 {
		logger.Log.Warn().Str("edge_id", edgeID).Int("dropped", dropped).Msg("Edge reported guardrail exception hits outside its snapshot; ignored")
	}
}
