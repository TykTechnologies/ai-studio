package agenttracing

import (
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/services/audit"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/rs/zerolog/log"
)

// Deps is everything the Enterprise implementation needs from the host.
type Deps struct {
	DB     *gorm.DB
	Config config.AgentTracingConfig
	// LicenseValid reports whether the Enterprise licence is valid. Nil
	// counts as not valid.
	LicenseValid func() bool
	// Audit returns the audit trail (payload views are audited). It is a
	// getter because the API builds the audit service later; nil is fine.
	Audit func() audit.Service
	// NodeID identifies this Studio node in leases and logs.
	NodeID string
	// Version is reported in exported resource attributes.
	Version string
}

// FactoryFunc builds the Enterprise service. The enterprise submodule
// registers one from its init() when built with -tags enterprise.
type FactoryFunc func(deps Deps) Service

var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory is called by the Enterprise implementation's init().
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// IsEnterpriseAvailable reports whether the Enterprise implementation is linked in.
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}

// NewService applies the start gate and returns the service.
//
// Tracing runs only when the Enterprise factory is registered, the existing
// Enterprise licence is valid and AGENT_TRACING_ENABLED=true (no separate
// licence entitlement). Only then does it build the Enterprise service; the
// caller turns the process flag (agenttrace.Enabled) on once Start succeeded
// (see GateOpen). Otherwise the flag is off,
// the result answers queries with ErrEnterpriseFeature (Community) or
// ErrDisabled (Enterprise, gated), and nothing is captured or written.
func NewService(deps Deps) Service {
	agenttrace.SetEnabled(false)
	if enterpriseFactory == nil {
		if deps.Config.Enabled {
			log.Warn().Msg("AGENT_TRACING_ENABLED is set, but Agent Call Tracing is an Enterprise feature; it stays off in this build")
		}
		return communityService{err: ErrEnterpriseFeature}
	}
	if !deps.Config.Enabled {
		return communityService{err: ErrDisabled, enterprise: true}
	}
	if deps.LicenseValid == nil || !deps.LicenseValid() {
		log.Warn().Msg("AGENT_TRACING_ENABLED is set, but the Enterprise licence is not valid; Agent Call Tracing stays off")
		return communityService{err: ErrDisabled, enterprise: true}
	}
	svc := enterpriseFactory(deps)
	if svc == nil {
		log.Error().Msg("Agent Call Tracing: the Enterprise factory returned no service; tracing stays off")
		return communityService{err: ErrDisabled, enterprise: true}
	}
	return svc
}

// GateOpen reports whether NewService built the Enterprise service (the
// start gate passed). The caller starts it and only then enables capture,
// so capture is never on without a working service and its keys.
func GateOpen(svc Service) bool {
	_, inert := svc.(communityService)
	return svc != nil && !inert
}
