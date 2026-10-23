package studio

import (
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
)

// applyNetguard applies the parsed LLM_UPSTREAM_BLOCK_INTERNAL setting
// (config.AppConf.AllowInternalUpstreams) to pkg/netguard, which every
// outbound client consults, and warns once when internal addresses are
// reachable.
func applyNetguard(conf *config.AppConf) {
	netguard.SetBlockInternal(!conf.AllowInternalUpstreams)
	if msg := netguard.DisabledWarning(); msg != "" {
		logger.Warn(msg)
	}
}
