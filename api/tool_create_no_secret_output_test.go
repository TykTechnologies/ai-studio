package api

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	zlog "github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Creating a tool writes nothing that holds its auth_key to stdout, stderr or
// the loggers: createTool used to print the whole input (ba1).
func TestCreateTool_AuthKeyNotWrittenToOutput(t *testing.T) {
	api, _ := setupTestAPI(t)
	const secret = "tool-auth-key-ba1-do-not-print"

	r, w, err := os.Pipe()
	require.NoError(t, err)
	prevOut, prevErr := os.Stdout, os.Stderr
	prevStd, prevZ := log.Writer(), zlog.Logger
	os.Stdout, os.Stderr = w, w
	log.SetOutput(w)
	zlog.Logger = zlog.Logger.Output(w)
	captured := make(chan []byte)
	go func() { b, _ := io.ReadAll(r); captured <- b }()
	restore := func() {
		os.Stdout, os.Stderr = prevOut, prevErr
		log.SetOutput(prevStd)
		zlog.Logger = prevZ
	}
	defer restore()

	resp := performRequest(api.router, "POST", "/api/v1/tools", map[string]any{
		"data": map[string]any{"type": "tools", "attributes": map[string]any{
			"name": "PrintCheck", "tool_type": models.ToolTypeREST, "oas_spec": testToolSpec,
			"auth_key": secret, "auth_schema_name": "bearerAuth",
		}},
	})
	restore()
	w.Close()
	out := <-captured

	assert.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	assert.False(t, bytes.Contains(out, []byte(secret)), "auth_key written to output:\n%s", out)
}
