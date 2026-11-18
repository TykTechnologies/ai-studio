package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactConnString(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"$SECRET/PG_CONN", "$SECRET/PG_CONN"},
		{"$ENV/PG_CONN", "$ENV/PG_CONN"},
		{"postgres://user:s3cret@db.internal:5432/vectors?sslmode=disable", "postgres://user:[redacted]@db.internal:5432/vectors?sslmode=disable"},
		{"redis://:s3cret@cache:6379/0", "redis://:[redacted]@cache:6379/0"},
		{"postgres://user@db/vectors", "postgres://user@db/vectors"},
		{"http://qdrant:6333", "http://qdrant:6333"},
		{"https://store.example/v1?api_key=abc&region=eu", "https://store.example/v1?api_key=[redacted]&region=eu"},
		{"host=db user=app password=s3cret dbname=v sslmode=disable", "host=db user=app password=[redacted] dbname=v sslmode=disable"},
		{"host=db password='s 3cret' dbname=v", "host=db password=[redacted] dbname=v"},
		{"Server=db;Pwd=s3cret;Database=v", "Server=db;Pwd=[redacted];Database=v"},
		{"postgres://user:p%zz@db/x", CredentialRedacted},
	} {
		got := RedactConnString(tc.in)
		assert.Equal(t, tc.want, got, tc.in)
		assert.NotContains(t, got, "s3cret", tc.in)
	}
}

func TestRestoreConnString(t *testing.T) {
	stored := "postgres://user:s3cret@db:5432/v"
	redacted := RedactConnString(stored)

	got, ok := RestoreConnString(redacted, stored)
	assert.True(t, ok)
	assert.Equal(t, stored, got, "the redacted form means unchanged")

	got, ok = RestoreConnString("postgres://user:new@db:5432/v", stored)
	assert.True(t, ok)
	assert.Equal(t, "postgres://user:new@db:5432/v", got, "a new value is taken")

	_, ok = RestoreConnString("postgres://user:[redacted]@evil.example:5432/v", stored)
	assert.False(t, ok, "the stored password never follows a changed host")

	got, ok = RestoreConnString("$SECRET/PG", stored)
	assert.True(t, ok)
	assert.Equal(t, "$SECRET/PG", got)

	got, ok = RestoreConnString("", stored)
	assert.True(t, ok)
	assert.Equal(t, "", got, "clearing is allowed")
}
