//go:build enterprise

package main

// The enterprise features are registered by enterprise_entlink.go, which
// `make ent-link` links in from enterprise/_coretests: this module does not
// require the private enterprise module (enterprise builds resolve it through
// enterprise.work). Without the link this fails to compile, rather than
// building a Community Edition binary under the enterprise tag.
var _ = enterpriseFeaturesLinked
