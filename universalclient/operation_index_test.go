package universalclient

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multiMethodSpec exercises every HTTP verb a path item can carry, so the index
// has to record the right method for each of them.
const multiMethodSpec = `{
  "openapi": "3.0.0",
  "info": {"title": "Verbs", "version": "1.0.0"},
  "servers": [{"url": "https://api.example.com"}],
  "paths": {
    "/things": {
      "get":    {"operationId": "listThings",   "responses": {"200": {"description": "ok"}}},
      "post":   {"operationId": "createThing",  "responses": {"200": {"description": "ok"}}},
      "put":    {"operationId": "replaceThing", "responses": {"200": {"description": "ok"}}},
      "delete": {"operationId": "deleteThing",  "responses": {"200": {"description": "ok"}}},
      "patch":  {"operationId": "patchThing",   "responses": {"200": {"description": "ok"}}},
      "head":   {"operationId": "headThing",    "responses": {"200": {"description": "ok"}}},
      "options":{"operationId": "optionsThing", "responses": {"200": {"description": "ok"}}}
    },
    "/things/{id}": {
      "get": {"operationId": "getThing", "responses": {"200": {"description": "ok"}}}
    }
  }
}`

// The index has to resolve every operation to the same path and method the
// document declares it under, across all path items.
func TestOperationIndexResolvesEveryMethod(t *testing.T) {
	client, err := NewClient([]byte(multiMethodSpec), "")
	require.NoError(t, err)

	cases := []struct {
		operationID string
		method      string
		path        string
	}{
		{"listThings", http.MethodGet, "/things"},
		{"createThing", http.MethodPost, "/things"},
		{"replaceThing", http.MethodPut, "/things"},
		{"deleteThing", http.MethodDelete, "/things"},
		{"patchThing", http.MethodPatch, "/things"},
		{"headThing", http.MethodHead, "/things"},
		{"optionsThing", http.MethodOptions, "/things"},
		{"getThing", http.MethodGet, "/things/{id}"},
	}

	for _, tc := range cases {
		t.Run(tc.operationID, func(t *testing.T) {
			operation, path, method, err := client.findOperation(tc.operationID)
			require.NoError(t, err)
			assert.Equal(t, tc.operationID, operation.OperationId)
			assert.Equal(t, tc.method, method)
			assert.Equal(t, tc.path, path)
		})
	}
}

func TestOperationIndexUnknownOperation(t *testing.T) {
	client, err := NewClient([]byte(multiMethodSpec), "")
	require.NoError(t, err)

	_, _, _, err = client.findOperation("noSuchOperation")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operation not found: noSuchOperation")
}

// The index is what keeps a lookup off the O(number of operations) scan the
// document walk used to do, so it has to survive the first call: building an
// MCP server calls findOperation once per operation, twice over.
func TestOperationIndexBuiltOnce(t *testing.T) {
	client, err := NewClient([]byte(multiMethodSpec), "")
	require.NoError(t, err)

	first, err := client.operationLocations()
	require.NoError(t, err)
	second, err := client.operationLocations()
	require.NoError(t, err)

	// Both calls handed back the same map, so the second one did not rescan.
	first["sentinel"] = &operationLocation{}
	assert.Contains(t, second, "sentinel")
}

// The handlers an MCP server registers capture the client and can run at the
// same time, so the lazily built index must be safe to race on.
func TestOperationIndexConcurrentLookups(t *testing.T) {
	client, err := NewClient([]byte(multiMethodSpec), "")
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, path, method, err := client.findOperation("getThing")
			assert.NoError(t, err)
			assert.Equal(t, http.MethodGet, method)
			assert.Equal(t, "/things/{id}", path)
		}()
	}
	wg.Wait()
}

// A lookup used to walk every path item, so cost grew with the size of the
// spec. Benchmark against a deliberately large document to keep that honest.
func BenchmarkFindOperationLargeSpec(b *testing.B) {
	paths := ""
	const operationCount = 500
	for i := 0; i < operationCount; i++ {
		if i > 0 {
			paths += ","
		}
		paths += fmt.Sprintf(
			`"/resource%d": {"get": {"operationId": "getResource%d", "responses": {"200": {"description": "ok"}}}}`,
			i, i,
		)
	}
	spec := fmt.Sprintf(`{
  "openapi": "3.0.0",
  "info": {"title": "Large", "version": "1.0.0"},
  "servers": [{"url": "https://api.example.com"}],
  "paths": {%s}
}`, paths)

	client, err := NewClient([]byte(spec), "")
	require.NoError(b, err)

	// The last operation is the worst case for the old linear scan.
	target := fmt.Sprintf("getResource%d", operationCount-1)
	_, _, _, err = client.findOperation(target)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, err := client.findOperation(target); err != nil {
			b.Fatal(err)
		}
	}
}
