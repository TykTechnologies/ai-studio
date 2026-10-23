package plugin_security

import (
	"fmt"
	"strings"
)

// PluginSource is a plugin deployment type, as an installation's source
// policy names it.
type PluginSource string

const (
	// SourceOCI is a plugin pulled from an OCI registry (oci://).
	SourceOCI PluginSource = "oci"
	// SourceFile is a local executable: a path or file:// command. Every
	// command that is neither oci:// nor grpc:// is run as one, so it is
	// this source too, including http:// and https:// commands.
	SourceFile PluginSource = "file"
	// SourceGRPC is an external plugin process reached over gRPC (grpc://).
	SourceGRPC PluginSource = "grpc"
)

// allSources lists every source in the order Allowed reports them.
var allSources = []PluginSource{SourceOCI, SourceFile, SourceGRPC}

// SourceOf classifies a plugin command exactly as the plugin managers
// dispatch it: a case-sensitive "oci://" prefix is oci, "grpc://" is grpc,
// and anything else is run as a local executable (file). The command is
// deliberately not trimmed, lower-cased or parsed, so " oci://x" or
// "OCI://x" is file, as it would be run.
func SourceOf(command string) PluginSource {
	switch {
	case strings.HasPrefix(command, "oci://"):
		return SourceOCI
	case strings.HasPrefix(command, "grpc://"):
		return SourceGRPC
	default:
		return SourceFile
	}
}

// SourcePolicy is the set of plugin sources an installation accepts. The
// zero value allows every source.
type SourcePolicy struct {
	allowed map[PluginSource]bool
	// setting names the configuration setting that changes the policy, for
	// error messages.
	setting string
}

// AllowAllSources returns the policy that accepts every source.
func AllowAllSources() SourcePolicy {
	return SourcePolicy{}
}

// ParseSourcePolicy parses a comma-separated list of sources ("oci",
// "file", "grpc"; case and spaces are ignored). An empty value allows every
// source. An unknown source, or a list with no sources (","), is an error:
// a typo must not silently allow everything or nothing.
func ParseSourcePolicy(csv string) (SourcePolicy, error) {
	if strings.TrimSpace(csv) == "" {
		return AllowAllSources(), nil
	}
	allowed := make(map[PluginSource]bool)
	for _, token := range strings.Split(csv, ",") {
		name := strings.ToLower(strings.TrimSpace(token))
		if name == "" {
			continue
		}
		source := PluginSource(name)
		if !isKnownSource(source) {
			return SourcePolicy{}, fmt.Errorf("unknown plugin source %q (valid sources: oci, file, grpc)", strings.TrimSpace(token))
		}
		allowed[source] = true
	}
	if len(allowed) == 0 {
		return SourcePolicy{}, fmt.Errorf("plugin source list %q names no source (valid sources: oci, file, grpc; leave it empty to allow all)", csv)
	}
	return SourcePolicy{allowed: allowed}, nil
}

func isKnownSource(source PluginSource) bool {
	for _, s := range allSources {
		if s == source {
			return true
		}
	}
	return false
}

// WithSetting returns the policy with the name of the setting that changes
// it (e.g. AI_STUDIO_PLUGIN_ALLOWED_SOURCES), which Check's error names.
func (p SourcePolicy) WithSetting(name string) SourcePolicy {
	p.setting = name
	return p
}

// AllowsAll reports whether the policy accepts every source.
func (p SourcePolicy) AllowsAll() bool {
	for _, s := range allSources {
		if !p.allows(s) {
			return false
		}
	}
	return true
}

// Allowed returns the accepted sources, in the order oci, file, grpc.
func (p SourcePolicy) Allowed() []PluginSource {
	var out []PluginSource
	for _, s := range allSources {
		if p.allows(s) {
			out = append(out, s)
		}
	}
	return out
}

func (p SourcePolicy) allows(source PluginSource) bool {
	return p.allowed == nil || p.allowed[source]
}

// Check returns an *ErrPluginSourceNotAllowed if the policy does not accept
// the command's source (see SourceOf).
func (p SourcePolicy) Check(command string) error {
	source := SourceOf(command)
	if p.allows(source) {
		return nil
	}
	return &ErrPluginSourceNotAllowed{Source: source, Allowed: p.Allowed(), Setting: p.setting}
}

// String describes the policy, e.g. "oci,file" or "all".
func (p SourcePolicy) String() string {
	if p.AllowsAll() {
		return "all"
	}
	names := make([]string, 0, len(allSources))
	for _, s := range p.Allowed() {
		names = append(names, string(s))
	}
	return strings.Join(names, ",")
}
