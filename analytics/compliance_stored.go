package analytics

import (
	"sync/atomic"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// storedHook is called with each batch of compliance events the database
// handler stored, after the insert (their IDs are set). Studio sets it to
// raise guardrail alerts (services.Service.PublishGuardrailAlerts); edges,
// which record through their own handler, never do.
var storedHook atomic.Pointer[func([]*models.ComplianceEvent)]

// SetComplianceEventsStored sets the hook; nil removes it.
func SetComplianceEventsStored(fn func([]*models.ComplianceEvent)) {
	if fn == nil {
		storedHook.Store(nil)
		return
	}
	storedHook.Store(&fn)
}

func complianceEventsStored(events []*models.ComplianceEvent) {
	if fn := storedHook.Load(); fn != nil && len(events) > 0 {
		(*fn)(events)
	}
}
