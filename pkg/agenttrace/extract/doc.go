// Package extract turns LLM request and response bodies into the correlation
// extracts of Agent Call Tracing (spec §4.4): the tool calls a response
// requests, the trailing tool results a request sends back, response ids and
// finish reasons. It also holds the canonicalisation (RFC 8785 JCS, §4.4 text
// normalisation) and the fingerprint helpers both sides of a correlation
// match must share, so an LLM's tool call and the executed MCP or REST call
// fingerprint equal.
//
// Nothing raw leaves this package: arguments and results come back only as
// HMAC fingerprints from the installed agenttrace.Fingerprinter, whatever the
// content mode. The library is pure (no I/O, no globals besides the
// Fingerprinter the caller passes); callers parse each body once and keep the
// result on the request's recorder.
package extract
