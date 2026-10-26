package sqlitetemplate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

type widget struct {
	ID   uint
	Name string
}

func openMemory(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// The template is migrated once, and each copy is a database of its own:
// rows written to one copy are not in the template or in another copy.
func TestCopiesAreIndependentAndMigrateRunsOnce(t *testing.T) {
	migrations := 0
	tp := New(func(db *gorm.DB) error {
		migrations++
		if err := db.AutoMigrate(&widget{}); err != nil {
			return err
		}
		return db.Create(&widget{Name: "seed"}).Error
	})

	a, b := openMemory(t), openMemory(t)
	require.NoError(t, tp.CopyInto(a))
	require.NoError(t, tp.CopyInto(b))
	require.Equal(t, 1, migrations)

	require.NoError(t, a.Create(&widget{Name: "only in a"}).Error)

	var inA, inB []widget
	require.NoError(t, a.Order("id").Find(&inA).Error)
	require.NoError(t, b.Order("id").Find(&inB).Error)
	require.Len(t, inA, 2)
	require.Equal(t, []widget{{ID: 1, Name: "seed"}}, inB)

	c := openMemory(t)
	require.NoError(t, tp.CopyInto(c))
	var inC []widget
	require.NoError(t, c.Find(&inC).Error)
	require.Equal(t, []widget{{ID: 1, Name: "seed"}}, inC, "the template is unchanged by writes to a copy")
}
