package services

import "github.com/TykTechnologies/midsommar/v2/services/group_access"

// autoAddToDefaultCatalogue reports whether a newly saved LLM, data source or
// tool should be put into the Default catalogue when it is in no catalogue.
//
// The Default group owns the Default catalogues and every user joins the
// Default group, so anything in a Default catalogue is visible to everyone.
// That is the right outcome only in Community Edition, where group filtering
// is compiled out and Default is the sole route by which an object reaches
// the portal at all. In Enterprise builds catalogues are the access control:
// an administrator grants each object to the teams that should see it, and an
// automatic grant to everyone would silently undo that on every save. So the
// auto-add is a CE-only convenience, keyed off the same build-time switch
// that enables group filtering.
//
// Users still auto-join the Default group and the Default group still owns
// the Default catalogues in both editions; only the object-side auto-add is
// edition-specific.
func autoAddToDefaultCatalogue() bool {
	return !group_access.IsFilteringEnabled()
}
