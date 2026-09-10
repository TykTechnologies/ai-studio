package governed_metadata

import (
	"gorm.io/gorm"
)

// FactoryFunc creates a governed metadata service instance.
// Enterprise code registers its factory via init().
type FactoryFunc func(db *gorm.DB, deps Deps) Service

// enterpriseFactory holds the enterprise implementation factory if available.
// Set by the enterprise submodule's init() when building with -tags enterprise.
var enterpriseFactory FactoryFunc

// RegisterEnterpriseFactory allows the enterprise implementation to register itself.
func RegisterEnterpriseFactory(f FactoryFunc) {
	enterpriseFactory = f
}

// NewService creates a governed metadata service.
// Returns the enterprise implementation when available, otherwise the CE stub.
func NewService(db *gorm.DB, deps Deps) Service {
	if enterpriseFactory != nil {
		return enterpriseFactory(db, deps)
	}
	return newCommunityService()
}

// IsEnterpriseAvailable reports whether the enterprise implementation is registered.
func IsEnterpriseAvailable() bool {
	return enterpriseFactory != nil
}

// BuiltinObjectTypes lists the object types every edition knows about.
func BuiltinObjectTypes() []ObjectTypeInfo {
	return []ObjectTypeInfo{
		{Slug: "llm", Label: "LLM", Source: "builtin"},
		{Slug: "tool", Label: "Tool", Source: "builtin"},
		{Slug: "datasource", Label: "Datasource", Source: "builtin"},
	}
}
