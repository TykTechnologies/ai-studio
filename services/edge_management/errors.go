package edge_management

import "errors"

// ErrEnterpriseFeature is returned when attempting to use enterprise-only features in CE
var ErrEnterpriseFeature = errors.New("multi-tenant namespaces require Enterprise Edition")
