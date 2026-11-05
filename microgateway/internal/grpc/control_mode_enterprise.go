//go:build enterprise

package grpc

// controlModeDefaultNamespaceOnly: Enterprise builds serve only the default
// namespace from a microgateway control plane; licensed namespaces are
// admitted by AI Studio.
const controlModeDefaultNamespaceOnly = true
