package edge_management

// Service defines the interface for edge management operations
// Community Edition: Single "default" namespace only
// Enterprise Edition: Full multi-tenant namespace support
type Service interface {
	// GetNamespaceForEdge returns the namespace to use for edge registration
	// CE: Always returns "default" (silent enforcement)
	// ENT: Returns requested namespace or "default" if empty
	GetNamespaceForEdge(requested string) string

	// ListNamespaces returns all active namespaces
	// CE: Returns ["default"]
	// ENT: Returns all distinct namespaces from database
	ListNamespaces() ([]string, error)

	// GetNamespaceStats returns statistics for a namespace
	// CE: Returns error - enterprise-only feature
	// ENT: Returns edge count, connection status, etc.
	GetNamespaceStats(namespace string) (map[string]interface{}, error)
}
