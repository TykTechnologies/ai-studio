package edge_management

// FactoryRegistered reports whether the enterprise implementation registered
// itself.
func FactoryRegistered() bool {
	return enterpriseFactory != nil
}
