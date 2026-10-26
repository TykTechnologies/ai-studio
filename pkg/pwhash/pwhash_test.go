package pwhash

import (
	"crypto/fips140"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bcryptFixture is bcrypt("correct horse", cost 10), as standard builds
// have always written it.
const bcryptFixture = "$2a$10$Q28z2xyQnAaw3y7XgwQEF.DQtiAfhYrYUDv4tJBI8E3l.tNo6WncG"

// pbkdf2Fixture is PBKDF2-HMAC-SHA256("correct horse", salt 00..0f, 1000
// iterations, 32 bytes).
const pbkdf2Fixture = "$pbkdf2-sha256$1000$AAECAwQFBgcICQoLDA0ODw$yRTMTwbMbo9G0VfjobWqerzuuxe7BETNTErBbKKumGQ"

func TestHash_StandardModeWritesBcrypt(t *testing.T) {
	if fips140.Enabled() {
		t.Skip("standard-mode writer; this process runs in FIPS mode")
	}
	h, err := Hash("correct horse")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(h, "$2"), "standard builds keep writing bcrypt, got %q", h)
	assert.True(t, Verify("correct horse", h))
	assert.False(t, NeedsUpgrade(h))
}

func TestHash_FIPSModeWritesPBKDF2(t *testing.T) {
	ForceFIPSModeForTest(t)
	h, err := Hash("correct horse")
	require.NoError(t, err)
	require.True(t, IsPBKDF2(h), "got %q", h)
	parts := strings.Split(strings.TrimPrefix(h, PBKDF2Prefix), "$")
	require.Len(t, parts, 3)
	assert.Equal(t, "1000", parts[0], "test binaries use the low iteration count")
	salt, err := b64.DecodeString(parts[1])
	require.NoError(t, err)
	assert.Len(t, salt, 16)
	key, err := b64.DecodeString(parts[2])
	require.NoError(t, err)
	assert.Len(t, key, 32)

	assert.True(t, Verify("correct horse", h))
	assert.False(t, Verify("wrong horse", h))
	assert.False(t, NeedsUpgrade(h))

	again, err := Hash("correct horse")
	require.NoError(t, err)
	assert.NotEqual(t, h, again, "random salt")
}

func TestVerify_BothFormatsInEveryMode(t *testing.T) {
	check := func(t *testing.T) {
		assert.True(t, Verify("correct horse", bcryptFixture))
		assert.False(t, Verify("wrong", bcryptFixture))
		assert.True(t, Verify("correct horse", pbkdf2Fixture))
		assert.False(t, Verify("wrong", pbkdf2Fixture))
	}
	t.Run("standard", check)
	t.Run("fips", func(t *testing.T) {
		ForceFIPSModeForTest(t)
		check(t)
	})
}

func TestNeedsUpgrade_OnlyBcryptInFIPSMode(t *testing.T) {
	if !fips140.Enabled() {
		assert.False(t, NeedsUpgrade(bcryptFixture), "standard builds never rewrite")
	}
	ForceFIPSModeForTest(t)
	assert.True(t, NeedsUpgrade(bcryptFixture))
	assert.False(t, NeedsUpgrade(pbkdf2Fixture))
}

func TestVerify_MalformedNeverMatches(t *testing.T) {
	for _, h := range []string{
		"",
		"plaintext",
		"correct horse", // a stored plaintext never matches itself
		PBKDF2Prefix,
		PBKDF2Prefix + "1000$AAECAwQFBgcICQoLDA0ODw",
		PBKDF2Prefix + "0$AAECAwQFBgcICQoLDA0ODw$yRTMTwbMbo9G0VfjobWqerzuuxe7BETNTErBbKKumGQ",
		PBKDF2Prefix + "99999999999$AAECAwQFBgcICQoLDA0ODw$yRTMTwbMbo9G0VfjobWqerzuuxe7BETNTErBbKKumGQ",
		PBKDF2Prefix + "1000$!!$yRTMTwbMbo9G0VfjobWqerzuuxe7BETNTErBbKKumGQ",
		PBKDF2Prefix + "1000$AAECAwQFBgcICQoLDA0ODw$",
		"$2a$10$short",
	} {
		assert.False(t, Verify("correct horse", h), "%q", h)
	}
}
