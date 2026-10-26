package models

import (
	"crypto/fips140"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Hash test passwords at bcrypt.MinCost instead of DefaultCost (~0.8 s per
// hash under -race). See UseMinPasswordCostForTests.
func init() { UseMinPasswordCostForTests() }

// bcryptCost5Password1 is "Password1!" hashed with bcrypt at cost 5
// (MinCost+1), a fixture so this test needs no bcrypt import (pkg/pwhash is
// the only package that has one).
const bcryptCost5Password1 = "$2a$05$Fs/RKPiMriDcJNTBQZT25.o4LSfF4SBs7pS.uegQ8REcJ9Mt9cpCe"

// The test binary hashes at MinCost, and a hash made at another cost (every
// password a production Studio stored is DefaultCost) still verifies: bcrypt
// reads the cost from the hash, not from PasswordHashCost. MinCost+1 shows
// that without paying for a DefaultCost hash (~2.5 s under -race).
func TestPasswordHashCost(t *testing.T) {
	require.Equal(t, 4, PasswordHashCost(), "bcrypt.MinCost")

	require.True(t, IsPasswordValid("Password1!", bcryptCost5Password1))
	require.False(t, IsPasswordValid("Password2!", bcryptCost5Password1))

	if fips140.Enabled() {
		t.Skip("FIPS 140-3 mode hashes new passwords with PBKDF2 (pkg/pwhash)")
	}
	hashed, err := HashPassword("Password1!")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hashed, "$2a$04$"), "a MinCost bcrypt hash: %s", hashed)
}
