package agenttracing

import "context"

// communityService is the inert service: the Community stub, and the
// Enterprise build when the start gate is closed. Queries (added by the API
// beads) return err.
type communityService struct {
	err        error
	enterprise bool
}

func (communityService) Start(context.Context) error { return nil }
func (communityService) Stop()                       {}
func (communityService) Running() bool               { return false }

func (s communityService) IsEnterpriseAvailable() bool { return s.enterprise }

// Err is the error the inert service answers queries with.
func (s communityService) Err() error { return s.err }
