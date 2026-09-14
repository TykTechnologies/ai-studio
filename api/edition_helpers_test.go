package api

import "github.com/TykTechnologies/midsommar/v2/services/group_access"

// defaultCatalogueAutoAdds is the number of catalogues a freshly created
// object lands in with no catalogue chosen: 1 (Default) in Community Edition,
// where the group filter is compiled out and Default is the only route to the
// portal, and 0 in Enterprise builds, where catalogues are the access
// control. Tests that count catalogue memberships use it so they hold under
// both build tags.
func defaultCatalogueAutoAdds() int {
	if group_access.IsFilteringEnabled() {
		return 0
	}
	return 1
}
