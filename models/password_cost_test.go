package models

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// Hash test passwords at bcrypt.MinCost instead of DefaultCost (~0.8 s per
// hash under -race). See UseMinPasswordCostForTests.
func init() { UseMinPasswordCostForTests() }

// The test binary hashes at MinCost, and a hash made at another cost (every
// password a production Studio stored is DefaultCost) still verifies: bcrypt
// reads the cost from the hash, not from PasswordHashCost. MinCost+1 shows
// that without paying for a DefaultCost hash (~2.5 s under -race).
func TestPasswordHashCost(t *testing.T) {
	require.Equal(t, bcrypt.MinCost, PasswordHashCost())

	hashed, err := HashPassword("Password1!")
	require.NoError(t, err)
	cost, err := bcrypt.Cost([]byte(hashed))
	require.NoError(t, err)
	require.Equal(t, bcrypt.MinCost, cost)

	prod, err := bcrypt.GenerateFromPassword([]byte("Password1!"), bcrypt.MinCost+1)
	require.NoError(t, err)
	require.True(t, IsPasswordValid("Password1!", string(prod)))
	require.False(t, IsPasswordValid("Password2!", string(prod)))
}
