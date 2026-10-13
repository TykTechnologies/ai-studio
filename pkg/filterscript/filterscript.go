// Package filterscript holds the filter-script settings that both AI Studio
// and the microgateway check at startup.
package filterscript

import (
	"os"
	"strings"
)

// EnvScriptAllowOS is the switch that gives filter scripts Tengo's "os"
// module: environment variables (TYK_AI_SECRET_KEY among them), the file
// system and process execution. The Enterprise script runner reads it from
// the environment only; it is off unless set.
const EnvScriptAllowOS = "FILTER_SCRIPT_ALLOW_OS"

// OSModuleWarning is logged at startup when EnvScriptAllowOS is on.
const OSModuleWarning = "SECURITY WARNING: FILTER_SCRIPT_ALLOW_OS is enabled. Filter scripts can read this process's environment variables " +
	"(including TYK_AI_SECRET_KEY), read and write files and run processes on this host. Unset it unless a script needs Tengo's os module."

// OSModuleAllowed reports whether EnvScriptAllowOS is on, read the way the
// Enterprise runner reads it ("1", "true", "yes" or "on", in any case).
func OSModuleAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvScriptAllowOS))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
