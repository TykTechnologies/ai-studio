package secrets

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Visor review follow-up: concurrent cache misses for the same (secret, salt)
// pair must coalesce into a single scrypt computation (no thundering herd).

func TestDeriveKeyScryptCoalescesConcurrentMisses(t *testing.T) {
	salt := []byte("stampede-salt-16") // unique to this test; cold cache
	before := scryptComputations.Load()

	const goroutines = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([][]byte, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = deriveKeyScrypt("stampede-password", salt)
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 0; i < goroutines; i++ {
		require.NoError(t, errs[i])
		assert.Equal(t, results[0], results[i], "all callers must receive the same key")
	}

	computed := scryptComputations.Load() - before
	assert.EqualValues(t, 1, computed, "concurrent misses for the same key must run scrypt exactly once")
}
