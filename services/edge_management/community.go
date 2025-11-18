//go:build !enterprise
// +build !enterprise

package edge_management

// communityService is the Community Edition implementation
// Forces all edges to "default" namespace without logging
type communityService struct{}

// newCommunityService creates a new community edition service
func newCommunityService() Service {
	return &communityService{}
}

// GetNamespaceForEdge always returns "default" in Community Edition
// No logging - silent enforcement
func (s *communityService) GetNamespaceForEdge(requested string) string {
	return "default"
}

// ListNamespaces returns only the "default" namespace in Community Edition
func (s *communityService) ListNamespaces() ([]string, error) {
	return []string{"default"}, nil
}

// GetNamespaceStats is not available in Community Edition
func (s *communityService) GetNamespaceStats(namespace string) (map[string]interface{}, error) {
	return nil, ErrEnterpriseFeature
}
