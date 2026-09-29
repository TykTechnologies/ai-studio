//go:build enterprise

package studio

import "testing"

func TestEnterpriseHostBuildHasNoCgoOnlyDependencies(t *testing.T) {
	checkNoCgoOnlyDependencies(t, "studio_noui enterprise")
}
