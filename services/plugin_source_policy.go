package services

import (
	"sync"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
)

// sourcePolicyHolder gives a plugin component the plugin source policy
// (AI_STUDIO_PLUGIN_ALLOWED_SOURCES). SetSourcePolicy overrides it (tests,
// embedders); otherwise the configured policy applies, read when checked, so
// a component built without the setter never fails open. An unset policy
// allows every source.
type sourcePolicyHolder struct {
	sourcePolicyMu sync.RWMutex
	sourcePolicyP  *plugin_security.SourcePolicy
}

// SetSourcePolicy sets the plugin source policy, overriding the configured one.
func (h *sourcePolicyHolder) SetSourcePolicy(policy plugin_security.SourcePolicy) {
	h.sourcePolicyMu.Lock()
	defer h.sourcePolicyMu.Unlock()
	h.sourcePolicyP = &policy
}

// sourcePolicy returns the policy set with SetSourcePolicy, else the
// configured one.
func (h *sourcePolicyHolder) sourcePolicy() plugin_security.SourcePolicy {
	h.sourcePolicyMu.RLock()
	defer h.sourcePolicyMu.RUnlock()
	if h.sourcePolicyP != nil {
		return *h.sourcePolicyP
	}
	return config.Get("").PluginSourcePolicy
}

// checkSource returns *plugin_security.ErrPluginSourceNotAllowed when the
// policy does not accept the command's deployment type.
func (h *sourcePolicyHolder) checkSource(command string) error {
	return h.sourcePolicy().Check(command)
}
