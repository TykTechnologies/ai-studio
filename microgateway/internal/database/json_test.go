package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// JSON keeps the behaviour of gorm.io/datatypes.JSON v1.2.6, which it
// replaced (checked side by side before the swap): an empty value is stored
// as NULL; a model loaded by gorm reads that NULL back empty (gorm does not
// call Scan for NULL), while a direct Scan of NULL gives the JSON value null;
// and it marshals as JSON rather than base64.
func TestJSONColumnBehaviour(t *testing.T) {
	db := openTestDB(t)

	withMeta := &App{Name: "with", IsActive: true, Metadata: JSON(`{"team":"a"}`)}
	empty := &App{Name: "empty", IsActive: true}
	require.NoError(t, db.Create(withMeta).Error)
	require.NoError(t, db.Create(empty).Error)

	var isNull bool
	require.NoError(t, db.Raw("SELECT metadata IS NULL FROM apps WHERE id = ?", empty.ID).Scan(&isNull).Error)
	assert.True(t, isNull, "an empty JSON is stored as NULL")

	var got App
	require.NoError(t, db.First(&got, withMeta.ID).Error)
	assert.JSONEq(t, `{"team":"a"}`, string(got.Metadata))

	var gotEmpty App
	require.NoError(t, db.First(&gotEmpty, empty.ID).Error)
	assert.Empty(t, gotEmpty.Metadata, "gorm leaves a NULL column's field empty")

	var scanned JSON
	require.NoError(t, db.Raw("SELECT metadata FROM apps WHERE id = ?", empty.ID).Row().Scan(&scanned))
	assert.Equal(t, JSON("null"), scanned, "a direct Scan of NULL gives the JSON value null")

	out, err := json.Marshal(struct{ M JSON }{JSON(`{"k":1}`)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"M":{"k":1}}`, string(out))

	var in struct{ M JSON }
	require.NoError(t, json.Unmarshal([]byte(`{"M":[1,2]}`), &in))
	assert.Equal(t, JSON(`[1,2]`), in.M)
}
