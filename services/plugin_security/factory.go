package plugin_security

// ServiceFactory is a function that creates a Service implementation
type ServiceFactory func(config *Config) Service

var (
	// enterpriseFactory holds the registered enterprise factory function
	enterpriseFactory ServiceFactory
)

// RegisterEnterpriseFactory registers the enterprise implementation factory
// This is called by the enterprise package's init() function
func RegisterEnterpriseFactory(factory ServiceFactory) {
	enterpriseFactory = factory
}

// NewService creates a new plugin security service
// Returns enterprise implementation if available, otherwise community stub
func NewService(config *Config) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(config)
	}
	return newCommunityService(config)
}

// IsEnterpriseAvailable returns true if enterprise plugin security features are available
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}
