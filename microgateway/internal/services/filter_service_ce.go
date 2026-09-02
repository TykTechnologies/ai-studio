//go:build !enterprise

package services

import (
	"log/slog"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
)

// runFilterScript always returns true in CE (enterprise feature)
func (s *FilterService) runFilterScript(filter *database.Filter, payload map[string]interface{}) (bool, error) {
	slog.Warn("⚠️ MICROGATEWAY CE: Filter execution skipped (enterprise feature)", "filter_name", filter.Name)
	return true, nil // Always allow in CE
}
