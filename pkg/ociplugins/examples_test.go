// pkg/ociplugins/examples_test.go
package ociplugins

import (
	"fmt"
)

// ExampleParseOCICommand demonstrates how to parse OCI plugin commands
func ExampleParseOCICommand() {
	// Basic OCI reference with digest
	ref, params, err := ParseOCICommand("oci://nexus.example.com/plugins/ner@sha256:0123deadbeef456")
	if err != nil {
		panic(err)
	}

	fmt.Printf("Registry: %s\n", ref.Registry)
	fmt.Printf("Repository: %s\n", ref.Repository)
	fmt.Printf("Digest: %s\n", ref.Digest)
	fmt.Printf("Architecture: %s\n", params.Architecture)

	// OCI reference with parameters
	_, params2, err := ParseOCICommand("oci://registry.com/plugins/test@sha256:abc123?arch=linux/arm64&pubkey=test.pub")
	if err != nil {
		panic(err)
	}

	fmt.Printf("Architecture: %s\n", params2.Architecture)
	fmt.Printf("Public Key: %s\n", params2.PublicKey)

	// Output:
	// Registry: nexus.example.com
	// Repository: plugins/ner
	// Digest: sha256:0123deadbeef456
	// Architecture: linux/amd64
	// Architecture: linux/arm64
	// Public Key: test.pub
}

// ExampleOCIPluginClient_FetchPlugin demonstrates how to fetch a plugin from an OCI registry
func ExampleOCIPluginClient_FetchPlugin() {
	// NOTE: This is a demonstration example - it won't work without a real registry

	// Create client configuration
	config := &OCIConfig{
		CacheDir:          "/tmp/plugin-cache",
		RequireSignature:  false, // Disable for demo
		AllowedRegistries: []string{"nexus.example.com"},
		Timeout:           30,
		RetryAttempts:     3,
	}

	// Create client
	_, err := NewOCIPluginClient(config)
	if err != nil {
		panic(err)
	}

	// Parse OCI reference
	ref, params, err := ParseOCICommand("oci://nexus.example.com/plugins/ner@sha256:0123deadbeef456")
	if err != nil {
		panic(err)
	}

	// This would fetch from the registry in a real scenario
	fmt.Printf("Would fetch plugin: %s\n", ref.FullReference())
	fmt.Printf("Target architecture: %s\n", params.Architecture)

	// Output:
	// Would fetch plugin: nexus.example.com/plugins/ner@sha256:0123deadbeef456
	// Target architecture: linux/amd64
}

// ExampleOCIReference_FullReference demonstrates reference formatting
func ExampleOCIReference_FullReference() {
	// Reference with digest
	ref1 := &OCIReference{
		Registry:   "registry.com",
		Repository: "plugins/test",
		Digest:     "sha256:abc123",
	}
	fmt.Printf("With digest: %s\n", ref1.FullReference())

	// Reference with tag
	ref2 := &OCIReference{
		Registry:   "registry.com",
		Repository: "plugins/test",
		Tag:        "v1.0.0",
	}
	fmt.Printf("With tag: %s\n", ref2.FullReference())

	// Reference without digest or tag (defaults to latest)
	ref3 := &OCIReference{
		Registry:   "registry.com",
		Repository: "plugins/test",
	}
	fmt.Printf("Default: %s\n", ref3.FullReference())

	// Output:
	// With digest: registry.com/plugins/test@sha256:abc123
	// With tag: registry.com/plugins/test:v1.0.0
	// Default: registry.com/plugins/test:latest
}