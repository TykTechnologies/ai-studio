package studio

import "github.com/TykTechnologies/midsommar/v2/secrets"

// Derive secret encryption keys at a test-only scrypt N instead of the real
// one (~0.5-1 s per derivation under -race). Values encrypted in this test
// binary only decrypt in it. See secrets.UseMinScryptCostForTests.
func init() { secrets.UseMinScryptCostForTests() }
