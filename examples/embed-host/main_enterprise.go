//go:build enterprise

// The Enterprise Edition of the demo host: build or run it with
// -tags enterprise (the enterprise submodule initialised, `make ent-link`
// run, and GOWORK=enterprise.work) and set TYK_AI_LICENSE. A host registers
// the enterprise features by importing enterprise/all from its main package;
// here that import is enterprise_entlink.go, linked in from
// enterprise/_coretests because this module does not require the private
// enterprise module. A host in its own module imports it directly.

package main

import "os"

// Fails to compile without enterprise_entlink.go, rather than building the
// Community Edition under the enterprise tag.
var _ = enterpriseFeaturesLinked

func init() {
	// The host supplies the licence; reading it on every call lets a renewed
	// licence take effect without a restart.
	licenseSource = func() string { return os.Getenv("TYK_AI_LICENSE") }
}
