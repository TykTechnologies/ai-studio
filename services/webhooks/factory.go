package webhooks

import (
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/services/audit"
	"gorm.io/gorm"
)

// Notifier is the slice of the notification service the feature needs. It is
// declared here rather than importing services to avoid the import cycle
// (services imports this package). *services.NotificationService satisfies it.
type Notifier interface {
	NotifyDirect(notificationID, notifType, title, content string, userFlags uint) error
}

// Deps is everything the enterprise implementation needs from the host.
type Deps struct {
	DB *gorm.DB
	// Bus is the in-process event bus. Nil means events are not available on
	// this node (targets can still be managed; nothing is ingested).
	Bus eventbridge.Bus
	// Notifier sends in-app/email notifications to administrators. May be nil.
	Notifier Notifier
	// Audit returns the audit trail service. It is a getter because the audit
	// service is created by the API after this service exists; nil is fine.
	Audit  func() audit.Service
	Config config.WebhooksConfig
	// NodeID identifies this Studio node in delivery leases and logs.
	NodeID string
	// Version is reported in the User-Agent of outbound requests.
	Version string
}

// FactoryFunc builds a webhooks service. The enterprise submodule registers
// one from its init() when built with -tags enterprise.
type FactoryFunc func(deps Deps) Service

var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory is called by the enterprise implementation's init().
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// NewService returns the enterprise implementation when registered, otherwise
// the community no-op.
func NewService(deps Deps) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(deps)
	}
	return newCommunityService()
}

// IsEnterpriseAvailable reports whether the enterprise implementation is linked in.
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}
