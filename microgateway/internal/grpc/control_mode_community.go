//go:build !enterprise

package grpc

// controlModeDefaultNamespaceOnly: Community builds serve every namespace.
const controlModeDefaultNamespaceOnly = false
