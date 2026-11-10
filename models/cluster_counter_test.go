package models

import (
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// A counter starts at the given first value, then goes up by one per draw,
// with no value handed out twice to replicas drawing at the same moment.
func checkClusterCounter(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	const first = int64(1)<<62 + 1
	v, err := NextClusterCounter(db, name, first)
	require.NoError(t, err)
	assert.Equal(t, first, v)
	v, err = NextClusterCounter(db, name, 7) // first only matters once
	require.NoError(t, err)
	assert.Equal(t, first+1, v)

	other, err := NextClusterCounter(db, name+".other", 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), other, "counters are independent")

	const workers, draws = 8, 25
	var mu sync.Mutex
	var got []int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < draws; i++ {
				v, err := NextClusterCounter(db, name, first)
				if !assert.NoError(t, err) {
					return
				}
				mu.Lock()
				got = append(got, v)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	require.Len(t, got, workers*draws)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	for i, v := range got {
		assert.Equal(t, first+2+int64(i), v)
	}
}

func TestNextClusterCounter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "c.db")+"?_busy_timeout=10000&_journal_mode=WAL"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ClusterCounter{}))
	checkClusterCounter(t, db, "test.counter")
}

func TestNextClusterCounter_PostgreSQL(t *testing.T) {
	db := setupPostgreSQLTestDB(t)
	name := "test.counter." + t.Name()
	clean := func() { db.Where("name LIKE ?", name+"%").Delete(&ClusterCounter{}) }
	clean()
	t.Cleanup(clean)
	checkClusterCounter(t, db, name)
}
