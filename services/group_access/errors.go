package group_access

import "errors"

var (
	// ErrEnterpriseFeature is returned when a Community Edition user attempts to use enterprise-only functionality
	ErrEnterpriseFeature = errors.New("group-based access control requires Enterprise Edition")

	// ErrAccessDenied is returned when a user doesn't have access to a resource
	ErrAccessDenied = errors.New("access denied: resource not available in user's groups")
)
