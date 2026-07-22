package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

func TestMergeMetadataPreservingRedacted(t *testing.T) {
	existing := models.JSONMap{
		"aws_access_key_id":     "AKIAREAL",
		"aws_secret_access_key": "realsecretvalue",
		"aws_session_token":     "oldtoken",
	}

	t.Run("preserves redacted fields, applies changed ones", func(t *testing.T) {
		incoming := models.JSONMap{
			"aws_access_key_id":     REDACTED_VALUE, // unchanged in the UI
			"aws_secret_access_key": "newsecret",    // user re-typed
			"aws_session_token":     REDACTED_VALUE, // unchanged
		}
		got := mergeMetadataPreservingRedacted(existing, incoming)
		if got["aws_access_key_id"] != "AKIAREAL" {
			t.Errorf("access key id: got %v, want preserved AKIAREAL", got["aws_access_key_id"])
		}
		if got["aws_secret_access_key"] != "newsecret" {
			t.Errorf("secret: got %v, want newsecret", got["aws_secret_access_key"])
		}
		if got["aws_session_token"] != "oldtoken" {
			t.Errorf("session token: got %v, want preserved oldtoken", got["aws_session_token"])
		}
	})

	t.Run("omitted field is dropped (e.g. cleared session token)", func(t *testing.T) {
		incoming := models.JSONMap{
			"aws_access_key_id":     REDACTED_VALUE,
			"aws_secret_access_key": REDACTED_VALUE,
			// aws_session_token intentionally omitted
		}
		got := mergeMetadataPreservingRedacted(existing, incoming)
		if _, present := got["aws_session_token"]; present {
			t.Errorf("session token should be dropped when omitted, got %v", got["aws_session_token"])
		}
		if got["aws_access_key_id"] != "AKIAREAL" {
			t.Errorf("access key id should be preserved, got %v", got["aws_access_key_id"])
		}
	})

	t.Run("redacted with no existing value is dropped, not stored", func(t *testing.T) {
		got := mergeMetadataPreservingRedacted(models.JSONMap{}, models.JSONMap{
			"aws_secret_access_key": REDACTED_VALUE,
		})
		if v, present := got["aws_secret_access_key"]; present {
			t.Errorf("placeholder must not be stored when no existing value, got %v", v)
		}
	})

	t.Run("nil incoming returns existing", func(t *testing.T) {
		got := mergeMetadataPreservingRedacted(existing, nil)
		if got["aws_access_key_id"] != "AKIAREAL" {
			t.Errorf("expected existing returned, got %v", got)
		}
	})
}
