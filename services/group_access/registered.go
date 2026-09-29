package group_access

// FactoryRegistered reports whether the enterprise implementation registered
// itself.
func FactoryRegistered() bool {
	return enterpriseFactory != nil
}
