package scripting

import (
	"sync"

	"github.com/TykTechnologies/midsommar/v2/services"
)

// ScriptRunner manages script execution with thread safety
type ScriptRunner struct {
	mu     sync.Mutex
	source []byte
}

// NewScriptRunner creates a new script runner
func NewScriptRunner(source []byte) *ScriptRunner {
	return &ScriptRunner{
		source: source,
	}
}

// ServiceInterface allows scripts to interact with system services
type ServiceInterface = services.ServiceInterface
