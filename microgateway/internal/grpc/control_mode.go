package grpc

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ControlModeDeprecationMessage is logged when a microgateway starts as a
// control plane (GATEWAY_MODE=control).
const ControlModeDeprecationMessage = "GATEWAY_MODE=control is deprecated and will be removed in a future release: " +
	"AI Studio is the supported control plane. Run edge gateways (GATEWAY_MODE=edge) against AI Studio instead."

// logControlModeDeprecation warns that control mode is deprecated.
func logControlModeDeprecation() {
	log.Warn().Str("gateway_mode", "control").Msg(ControlModeDeprecationMessage)
}

// checkControlModeNamespace refuses, in Enterprise builds, an edge in a
// namespace other than the default one: namespaces are licensed by AI
// Studio, which a microgateway control plane would bypass. Community builds
// accept every namespace.
func checkControlModeNamespace(ns string) error {
	if !controlModeDefaultNamespaceOnly || models.CanonicalNamespace(ns) == models.DefaultNamespace {
		return nil
	}
	return status.Errorf(codes.FailedPrecondition,
		"namespace %q is not served: the microgateway control mode (GATEWAY_MODE=control) is deprecated and "+
			"serves only the default namespace in Enterprise builds; register this edge against AI Studio, "+
			"the supported control plane", ns)
}
