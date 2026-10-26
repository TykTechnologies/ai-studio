package pluginenv

import (
	"crypto/fips140"
	"debug/buildinfo"
	"strings"

	"github.com/rs/zerolog/log"
)

// fipsEnabled is crypto/fips140.Enabled; tests replace it.
var fipsEnabled = fips140.Enabled

// FIPSBuild reports whether the Go binary at path is a FIPS 140-3 build:
// built against the validated module (GOFIPS140=v1.0.0) and defaulting to
// fips140=on (or only). why says
// what is missing when it is not, including that the file is not a readable
// Go binary (a plugin in another language cannot be told apart).
func FIPSBuild(path string) (ok bool, why string) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return false, "not a readable Go binary: " + err.Error()
	}
	var gofips, godebug string
	for _, s := range info.Settings {
		switch s.Key {
		case "GOFIPS140":
			gofips = s.Value
		case "DefaultGODEBUG":
			godebug = s.Value
		}
	}
	if gofips == "" || gofips == "off" {
		return false, "built without GOFIPS140"
	}
	if gofips != "v1.0.0" && !strings.HasPrefix(gofips, "v1.0.0-") {
		return false, "built with GOFIPS140=" + gofips + ", not the validated v1.0.0"
	}
	for _, kv := range strings.Split(godebug, ",") {
		if kv == "fips140=on" || kv == "fips140=only" {
			return true, ""
		}
	}
	return false, "does not default to fips140=on"
}

// warnIfNotFIPS logs a warning when this process runs in FIPS 140-3 mode and
// is about to start a plugin binary that is not a FIPS build: every plugin
// does its own cryptography, so the deployment is only FIPS compliant if its
// plugins are FIPS builds too. It only warns; the plugin still starts.
func warnIfNotFIPS(path string) {
	if !fipsEnabled() {
		return
	}
	if ok, why := FIPSBuild(path); !ok {
		log.Warn().Str("plugin", path).Str("reason", why).
			Msg("FIPS 140-3 mode: starting a plugin that is not a FIPS build; publish and use its FIPS variant (FIPS=1, tag <version>-fips)")
	}
}
