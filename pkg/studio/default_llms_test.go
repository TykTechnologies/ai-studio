package studio

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// limitedLLMs allows max LLM creates, then refuses like the enterprise guard.
type limitedLLMs struct {
	entitlements.Service
	max, created int
}

func (l *limitedLLMs) GuardLLMCreate(tx *gorm.DB, fn func(*gorm.DB) error) error {
	if l.created >= l.max {
		return entitlements.NewExceededError("Your organisation has reached its licensed limit of 1 LLM.")
	}
	l.created++
	return tx.Transaction(fn)
}

// A licence limit below the number of defaults seeds what fits and does not
// stop start-up; a later start seeds nothing more.
func TestEnsureDefaultLLMsWithinLicence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/d.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	ent := &limitedLLMs{Service: entitlements.NewCommunityService(), max: 1}
	require.NoError(t, ensureDefaultLLMs(db, ent))
	require.NoError(t, ensureDefaultLLMs(db, ent))

	var n int64
	require.NoError(t, db.Model(&models.LLM{}).Count(&n).Error)
	assert.Equal(t, int64(1), n)
}
