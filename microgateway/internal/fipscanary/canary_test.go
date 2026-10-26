package fipscanary

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/fipstest"
)

// TestFIPSStrictCanary proves a make test-fips run of the microgateway
// module ran in strict mode; scripts/test-fips.sh requires it to pass.
func TestFIPSStrictCanary(t *testing.T) { fipstest.Canary(t) }
