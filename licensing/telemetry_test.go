package licensing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	t.Run("default URL", func(t *testing.T) {
		client := NewClient("")
		assert.Equal(t, telemetryAPIURL, client.URL)
		assert.NotNil(t, client.http)
		assert.Equal(t, 10*time.Second, client.http.Timeout)
	})

	t.Run("custom URL", func(t *testing.T) {
		customURL := "https://custom-telemetry.example.com"
		client := NewClient(customURL)
		assert.Equal(t, customURL, client.URL)
		assert.NotNil(t, client.http)
		assert.Equal(t, 10*time.Second, client.http.Timeout)
	})
}

func TestTrack(t *testing.T) {
	t.Run("successful track", func(t *testing.T) {
		// Create a test server that returns a 200 OK response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			assert.Equal(t, "/api/track", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Create a client using our test server URL
		client := NewClient(server.URL)

		// Call Track
		err := client.Track("test-identity", "test-event", map[string]interface{}{
			"key": "value",
		})

		// Verify no error
		assert.NoError(t, err)
	})

	t.Run("server error", func(t *testing.T) {
		// Create a test server that returns a 500 error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
		}))
		defer server.Close()

		// Create a client using our test server URL
		client := NewClient(server.URL)

		// Call Track
		err := client.Track("test-identity", "test-event", nil)

		// Verify error
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status code")
		assert.Contains(t, err.Error(), "500")
		assert.Contains(t, err.Error(), "server error")
	})

	t.Run("request error", func(t *testing.T) {
		// Create a client with invalid URL to force a request error
		client := NewClient("http://invalid-server-that-doesnt-exist.example")

		// Call Track
		err := client.Track("test-identity", "test-event", nil)

		// Verify error
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error sending event")
	})

	t.Run("event content verification", func(t *testing.T) {
		var capturedEvent Event

		// Create a test server that captures the event JSON
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			err := decoder.Decode(&capturedEvent)
			assert.NoError(t, err)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Create a client using our test server URL
		client := NewClient(server.URL)

		// Get current time to compare with
		beforeTime := time.Now().Unix()

		// Prepare properties
		props := map[string]interface{}{
			"key":    "value",
			"number": 42,
			"nested": map[string]interface{}{
				"inner": "data",
			},
		}

		// Call Track
		err := client.Track("test-identity", "test-event", props)

		// Get after time
		afterTime := time.Now().Unix()

		// Verify no error
		assert.NoError(t, err)

		// Verify timestamp is within range
		assert.GreaterOrEqual(t, capturedEvent.Timestamp, beforeTime)
		assert.LessOrEqual(t, capturedEvent.Timestamp, afterTime)

		// Verify other fields
		assert.Equal(t, "test-identity", capturedEvent.Identity)
		assert.Equal(t, "test-event", capturedEvent.Event)

		// For properties, verify individually due to JSON number conversion
		assert.Equal(t, "value", capturedEvent.Properties["key"])
		assert.Equal(t, float64(42), capturedEvent.Properties["number"]) // JSON unmarshals to float64
		nestedMap, ok := capturedEvent.Properties["nested"].(map[string]interface{})
		assert.True(t, ok, "nested property should be a map")
		assert.Equal(t, "data", nestedMap["inner"])
	})
}

func TestHashString(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		hash := HashString("")
		// SHA-256 of empty string: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
		expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		assert.Equal(t, expected, hash)
	})

	t.Run("non-empty string", func(t *testing.T) {
		hash := HashString("test")
		// SHA-256 of "test": 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
		expected := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
		assert.Equal(t, expected, hash)
	})

	t.Run("consistency check", func(t *testing.T) {
		// Hash the same string twice
		hash1 := HashString("test-consistency")
		hash2 := HashString("test-consistency")
		assert.Equal(t, hash1, hash2)
	})

	t.Run("manual SHA-256 verification", func(t *testing.T) {
		input := "verify-me"
		hash := HashString(input)

		// Calculate SHA-256 manually using crypto/sha256
		hasher := sha256.New()
		hasher.Write([]byte(input))
		expected := hex.EncodeToString(hasher.Sum(nil))

		assert.Equal(t, expected, hash)
	})
}
