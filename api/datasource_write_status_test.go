package api

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/services"
)

// A vector store this build cannot use (Chroma without cgo) is the caller's
// choice to fix, so it is a 400 rather than a 500.
func TestDatasourceWriteStatus(t *testing.T) {
	assert.Equal(t, http.StatusBadRequest, datasourceWriteStatus(fmt.Errorf("%w: chroma needs a build with cgo", services.ErrVectorStoreUnavailable)))
	assert.Equal(t, http.StatusBadRequest, datasourceWriteStatus(fmt.Errorf("x: %w", services.ErrEmbedderInvalid)))
	assert.Equal(t, http.StatusInternalServerError, datasourceWriteStatus(errors.New("database is locked")))
}
