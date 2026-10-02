package grpc

import (
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// endpointKey names one gateway endpoint carrying an auth plugin list.
type endpointKey struct {
	objectType string
	objectID   uint
}

// snapshotPluginAttachments reads, for the configuration snapshot, the order
// each LLM runs its plugins in and the auth plugin lists of the other
// endpoints, keeping only the plugins in sent (the active plugins the edge
// receives). A deactivated auth plugin so drops off an endpoint, as it drops
// off an LLM, and the endpoint goes back to app keys.
//
// A failed read leaves the snapshot without them, as a failed plugin preload
// does: the edge then keeps LLM plugins in the order it gets them and
// authenticates every other endpoint with app keys, as it did before these
// lists existed.
func (s *ControlServer) snapshotPluginAttachments(namespace string) (map[uint][]uint32, map[endpointKey][]uint32) {
	// The same plugin set the snapshot sends further down.
	sent := make(map[uint]bool)
	var sentIDs []uint
	q := s.db.Model(&models.Plugin{}).Where("is_active = ?", true)
	if namespace == "" {
		q = q.Where("namespace = ''")
	} else {
		q = q.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	if err := q.Pluck("id", &sentIDs).Error; err != nil {
		logger.Log.Warn().Err(err).Msg("Failed to read plugins for configuration snapshot attachments")
	}
	for _, id := range sentIDs {
		sent[id] = true
	}

	llmOrder := make(map[uint][]uint32)
	var llmPlugins []models.LLMPlugin
	if err := s.db.Where("is_active = ?", true).Order("llm_id ASC, order_index ASC").Find(&llmPlugins).Error; err != nil {
		logger.Log.Warn().Err(err).Msg("Failed to read LLM plugin order for configuration snapshot")
	}
	for _, lp := range llmPlugins {
		if !sent[lp.PluginID] {
			continue
		}
		llmOrder[lp.LLMID] = append(llmOrder[lp.LLMID], uint32(lp.PluginID))
	}

	endpointAuth := make(map[endpointKey][]uint32)
	rows, err := models.GetAllEndpointAuthPlugins(s.db)
	if err != nil {
		logger.Log.Warn().Err(err).Msg("Failed to read endpoint auth plugins for configuration snapshot")
	}
	for _, row := range rows {
		if !sent[row.PluginID] {
			continue
		}
		k := endpointKey{row.ObjectType, row.ObjectID}
		endpointAuth[k] = append(endpointAuth[k], uint32(row.PluginID))
	}
	return llmOrder, endpointAuth
}
