// Package nativeapi is the contract between LLM vendors and the native proxy
// routes (/llm/rest|stream|call/{slug}/...) for SDK-specific endpoints that do
// not fit the single synchronous "POST with a top-level model" shape:
// Anthropic Message Batches and count_tokens, OpenAI Responses ownership and
// OpenAI Batches and Files.
//
// The package is deliberately small and dependency-free: it imports only the
// standard library and models, never proxy, switches or vendors. Vendor code is
// declarative data plus small pure functions; it never calls the proxy. The
// proxy runs a fixed pipeline (auth, ACL, per-item model checks and filters,
// budget, ledger, ID wrapping, accounting) and supplies everything stateful.
//
// # Adding an extension to a vendor
//
//  1. Implement Provider on the vendor's LLMVendorProvider:
//
//     func (p *MyVendor) NativeAPI() nativeapi.Spec { return mySpec }
//
//     The proxy type-asserts Provider on switches.VendorMap[v](), the same way
//     it discovers IDecisionResponse. Build the Spec once (a package variable)
//     and return it; it must not change at runtime.
//
//  2. Declare every vendor-side object the gateway hands out an ID for as a
//     ResourceType. Its Name is namespaced by vendor ("anthropic.message_batch")
//     and is part of the signed ID, so an ID minted for one type never verifies
//     as another. IDCharset is an anchored regexp the raw vendor ID must match.
//     A non-nil Async marks the resource as long-running: it is written to the
//     native resource ledger, polled by the reconciler and accounted once.
//
//  3. Declare each endpoint as an Operation with its exact Method and canonical
//     vendor Path ("/v1/messages/batches/{id}/results"). Paths are matched
//     exactly, segment by segment; there is no prefix matching, so declaring
//     "/v1/messages/batches" never captures "POST /v1/messages". A {name}
//     segment matches one non-empty path segment; PathResource names the
//     ResourceType of the {id} segment so the proxy verifies and unwraps it
//     before the request leaves the gateway.
//
//  4. Fill the optional hooks the operation needs: Admit to split a body into
//     per-item admission units (model allow-list, request filters), Results for
//     JSONL result downloads (per-line response filters, usage), List for
//     ledger-served listings, RequestRefs/ResponseIDs for IDs embedded in
//     bodies, Transform to rewrite vendor URLs (never follow them), Upstream to
//     allow-list query parameters and headers, ErrorShape for vendor-shaped
//     errors.
//
//  5. For calls that stay on the synchronous chain but hand out or name
//     resource IDs (OpenAI's POST /v1/responses), declare a ChainHook: its
//     RequestRefs are verified and unwrapped before the filters run, its
//     ResponseIDs are wrapped in the JSON response and in the EventObject of
//     every server-sent event, and a created object its async type tracks
//     (AsyncSpec.Tracked) is ledgered. The vendor's usage analysis must then
//     skip that object.
//
//  6. Test the Spec with NewMatcher (it validates the Spec) and add proxy
//     harness tests for the end-to-end behaviour.
//
// # Signed resource IDs
//
// Every vendor ID returned to a client is wrapped by Codec as
//
//	<vendor_id>.t1k<kid>.<mac>
//
// where mac is a truncated HMAC-SHA256 over the length-prefixed resource type,
// vendor ID, LLM ID and App ID, and kid identifies the key. CodecForLLM keys it
// with HKDF(the LLM's hub-generated signing secret,
// "tyk-native-resource/secret/v1") and still accepts the LLM's previous key (a
// rotated secret, or the vendor-credential key HKDF(credential,
// "tyk-native-resource/v1") that signed IDs before secrets existed) until its
// grace period ends; an LLM without a secret signs with the vendor-credential
// key. Any replica, edge or the hub verifies an ID statelessly. Callers must
// answer every verification failure with the same 404 so the check is not an
// oracle.
//
// # JSONL results
//
// LineReader reads JSONL result files line by line with a bounded line size so
// multi-hundred-megabyte result files stream through the gateway without being
// buffered.
package nativeapi
