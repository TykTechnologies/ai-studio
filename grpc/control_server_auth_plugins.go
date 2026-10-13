package grpc

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// endpointKey names one gateway endpoint carrying an auth plugin list.
type endpointKey struct {
	objectType string
	objectID   uint
}

// snapshotPluginAttachments reads, for the configuration snapshot, the order
// each LLM runs its plugins in and the auth plugin lists of the other
// endpoints, keeping every active plugin. A deactivated plugin drops off an
// LLM, and a deactivated auth plugin off an endpoint, which goes back to app
// keys. An active plugin the snapshot does not send (one filed under another
// namespace) stays listed: edges leave out the LLM or endpoint that needs
// it (pkg/snapshotguard) rather than run it without the plugin.
//
// A failed read fails the snapshot: without these lists every LLM would go
// out without its plugins and every endpoint without its auth plugins, and
// edges could not tell. Edges keep their previous configuration instead.
func (s *ControlServer) snapshotPluginAttachments() (map[uint][]uint32, map[endpointKey][]uint32, error) {
	// Every active plugin, whichever namespace it is filed under: the
	// snapshot sends only this namespace's and the global ones.
	sent := make(map[uint]bool)
	var sentIDs []uint
	if err := s.db.Model(&models.Plugin{}).Where("is_active = ?", true).Pluck("id", &sentIDs).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to read plugins: %w", err)
	}
	for _, id := range sentIDs {
		sent[id] = true
	}

	llmOrder := make(map[uint][]uint32)
	var llmPlugins []models.LLMPlugin
	if err := s.db.Where("is_active = ?", true).Order("llm_id ASC, order_index ASC").Find(&llmPlugins).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to read LLM plugins: %w", err)
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
		return nil, nil, fmt.Errorf("failed to read endpoint auth plugins: %w", err)
	}
	for _, row := range rows {
		if !sent[row.PluginID] {
			continue
		}
		k := endpointKey{row.ObjectType, row.ObjectID}
		endpointAuth[k] = append(endpointAuth[k], uint32(row.PluginID))
	}
	return llmOrder, endpointAuth, nil
}
