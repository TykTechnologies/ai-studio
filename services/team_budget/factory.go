package team_budget

import (
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/services/audit"
	"gorm.io/gorm"
)

// Notifier is the slice of the notification service the feature needs. It is
// declared here rather than importing services to avoid the import cycle
// (services imports this package). *services.NotificationService satisfies it.
type Notifier interface {
	Notify(notificationID, title, templatePath string, data interface{}, userFlags uint) error
}

// Deps is everything the enterprise implementation needs from the host.
type Deps struct {
	DB *gorm.DB
	// Bus returns the in-process event bus, nil while none is wired. It is a
	// getter because the bus is attached after the service exists.
	Bus func() eventbridge.Bus
	// Notifier sends in-app/email notifications to administrators. May be nil.
	Notifier Notifier
	// Audit returns the audit trail service; nil is fine.
	Audit func() audit.Service
	// NodeID identifies this Studio node on the event bus.
	NodeID string
}

// FactoryFunc builds a team budget service. The enterprise submodule
// registers one from its init() when built with -tags enterprise.
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
