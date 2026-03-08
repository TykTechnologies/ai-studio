// Package all imports all secrets store implementations for side-effect registration.
// Import this package in main.go to register all available store backends:
//
//	import _ "github.com/TykTechnologies/midsommar/v2/secrets/all"
package all

import (
	_ "github.com/TykTechnologies/midsommar/v2/secrets/database"
	_ "github.com/TykTechnologies/midsommar/v2/secrets/nop"
)
