package licensing

import "time"

// Service defines the licensing service interface for microgateway
// Community Edition: Always valid, no checks
// Enterprise Edition: JWT validation, periodic checks
type Service interface {
	// Start initializes and starts the licensing service
	// ENT: Validates license at boot (exits if invalid)
	// ENT: Starts periodic validation checks (every 24h)
	Start() error

	// Stop gracefully stops all background processes
	Stop()

	// IsValid returns whether the license is currently valid
	// CE: Always returns true
	// ENT: Returns true if JWT is valid and not expired
	IsValid() bool

	// DaysLeft returns the number of days until license expiry
	// CE: Returns -1 (never expires)
	// ENT: Returns days until JWT exp claim
	DaysLeft() int

	// MaxLLMs returns the licence's LLM limit (max_llms), which a standalone
	// gateway enforces on its own LLMs. Edges get their LLMs from AI Studio,
	// which enforces its own licence.
	// CE: Always unlimited
	// ENT: The verified licence's limit; a licence without it allows 0
	MaxLLMs() (max int64, unlimited bool)
}

// Config holds licensing configuration
type Config struct {
	LicenseKey          string
	ValidityCheckPeriod time.Duration
}
