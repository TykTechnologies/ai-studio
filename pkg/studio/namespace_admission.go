package studio

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
)

// admissionInitiator is who a push to a newly licensed namespace is
// recorded as initiated by.
const admissionInitiator = "system: namespace licensed"

// pushAdmittedNamespaces pushes configuration to the edges of namespaces
// that were just given a licensed slot, so they configure themselves
// without reconnecting. The push is recorded in the database and
// delivered by whichever replica holds each edge's stream.
func pushAdmittedNamespaces(p *pushes.Coordinator) func([]string) {
	return func(namespaces []string) {
		for _, ns := range namespaces {
			_, err := p.Push(context.Background(), pushes.Request{Scope: pushes.ScopeNamespace, Namespace: ns, InitiatedBy: admissionInitiator})
			if err != nil && !errors.Is(err, pushes.ErrNoTargets) {
				logger.Warnf("Pushing configuration to newly licensed namespace %q failed: %v", ns, err)
			}
		}
	}
}
