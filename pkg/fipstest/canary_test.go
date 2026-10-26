package fipstest

import "testing"

// TestFIPSStrictCanary proves a make test-fips run of the root module ran in
// strict mode; scripts/test-fips.sh requires it to pass.
func TestFIPSStrictCanary(t *testing.T) { Canary(t) }
