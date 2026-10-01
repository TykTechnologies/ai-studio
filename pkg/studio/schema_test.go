//go:build !enterprise

package studio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// A database a full Studio migrated passes CheckSchema; an empty one does not.
func TestNewRecordsTheSchemaVersionCheckSchemaReads(t *testing.T) {
	opts := newTestOptions(t)
	ctx := context.Background()

	err := CheckSchema(ctx, opts.DB)
	assert.ErrorIs(t, err, ErrSchemaMissing, "nothing has migrated the database yet")

	s, err := New(opts)
	require.NoError(t, err)
	stopStudio(t, s)

	require.NoError(t, CheckSchema(ctx, opts.DB))
	var row models.StudioSchema
	require.NoError(t, opts.DB.Take(&row).Error)
	assert.Equal(t, models.SchemaVersion, row.Version)
	assert.Equal(t, models.MinReaderSchemaVersion, row.MinReaderVersion)
	assert.Equal(t, "test", row.WrittenBy)
}

// A newer schema that no longer lists this build as a reader is refused.
func TestCheckSchemaRefusesANewerSchemaItCannotRead(t *testing.T) {
	opts := newTestOptions(t)
	s, err := New(opts)
	require.NoError(t, err)
	stopStudio(t, s)

	require.NoError(t, opts.DB.Model(&models.StudioSchema{}).Where("1 = 1").
		Updates(map[string]interface{}{"version": models.SchemaVersion + 1, "min_reader_version": models.SchemaVersion + 1}).Error)
	assert.ErrorIs(t, CheckSchema(context.Background(), opts.DB), ErrSchemaTooNew)
}
