//go:build !enterprise

package services

// fullCredentialSyncBuild: full credential sync (EDGE_CREDENTIAL_SYNC=full)
// is an Enterprise option. A Community edge validates keys on demand.
const fullCredentialSyncBuild = false
