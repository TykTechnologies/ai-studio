// Package ociplugins fetches, verifies and caches plugins distributed as OCI
// artifacts.
//
// # Signature verifier contract
//
// Signatures are verified by a SecurityService (the Enterprise plugin
// security service; Community builds have no verifier). Three seams let it
// reach registries exactly as plugin pulls do:
//
//   - NewRemoteRepository builds the repository client, with the same
//     insecure-registry rules, registry auth and netguard SSRF guard as Pull.
//   - VerifySignature is called with a reference whose Digest is the
//     top-level digest the plugin reference resolved to (the image index
//     for a multi-arch plugin), which is the digest publish-plugin.sh signs.
//   - The registry auth config named by the plugin reference (?auth=) is
//     carried on the context: AuthConfigFrom(ctx) returns it, so the
//     verifier passes it to NewRemoteRepository.
package ociplugins
