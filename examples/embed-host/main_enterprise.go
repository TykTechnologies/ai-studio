//go:build enterprise

// The Enterprise Edition of the demo host: build or run it with
// -tags enterprise (the enterprise submodule initialised) and set
// TYK_AI_LICENSE. A host registers the enterprise features exactly as the
// standalone binary's main_enterprise.go does, by importing enterprise/all
// from its main package.

package main

import (
	"os"

	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/all" // Register every enterprise feature
)

func init() {
	// The host supplies the licence; reading it on every call lets a renewed
	// licence take effect without a restart.
	licenseSource = func() string { return os.Getenv("TYK_AI_LICENSE") }
}
