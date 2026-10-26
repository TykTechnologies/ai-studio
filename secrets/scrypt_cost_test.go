package secrets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// This package's own tests run at the real scrypt N (none calls
// UseMinScryptCostForTests). A value encrypted at the test N does not
// decrypt at the real one, and the other way round: N is not in the
// ciphertext, and the derived-key cache is keyed by N, so a cached key from
// one N is never used for the other.
func TestScryptCostIsPartOfTheKey(t *testing.T) {
	require.EqualValues(t, scryptN, scryptCostN.Load(), "this package's tests run at the production N")

	atReal, err := encrypt("key", "plain")
	require.NoError(t, err)

	scryptCostN.Store(scryptTestN)
	t.Cleanup(func() { scryptCostN.Store(scryptN) })

	atTest, err := encrypt("key", "plain")
	require.NoError(t, err)
	got, err := decrypt("key", atTest)
	require.NoError(t, err)
	require.Equal(t, "plain", got)
	_, err = decrypt("key", atReal)
	require.Error(t, err, "a value encrypted at the real N must not decrypt at the test N")

	scryptCostN.Store(scryptN)
	_, err = decrypt("key", atTest)
	require.Error(t, err, "a value encrypted at the test N must not decrypt at the real N")
	got, err = decrypt("key", atReal)
	require.NoError(t, err)
	require.Equal(t, "plain", got)
}
