package services

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func llmRequest(name string, active bool) *CreateLLMRequest {
	return &CreateLLMRequest{Name: name, Vendor: "openai", DefaultModel: "gpt-4", APIKey: "sk-test", IsActive: active}
}

func fixedLimit(max int64, unlimited bool) LLMLimitFunc {
	return func() (int64, bool) { return max, unlimited }
}

// Every LLM not deleted counts, inactive ones included; a refused create
// stores nothing and deleting an LLM frees its slot.
func TestCreateLLMWithinLicensedLimit(t *testing.T) {
	db, _, service := setupManagementTestDB(t)
	service.SetLLMLimit(fixedLimit(2, false))

	first, err := service.CreateLLM(llmRequest("one", true))
	require.NoError(t, err)
	_, err = service.CreateLLM(llmRequest("two", false))
	require.NoError(t, err, "an inactive LLM takes a slot too")

	_, err = service.CreateLLM(llmRequest("three", true))
	require.Error(t, err)
	assert.True(t, IsLLMLimitError(err))
	assert.Equal(t, "Your organisation has reached its licensed limit of 2 LLMs. Remove an LLM to make room, or contact your administrator to increase the quota.", err.Error())
	n, err := countLLMs(db)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	require.NoError(t, service.DeleteLLM(first.ID))
	_, err = service.CreateLLM(llmRequest("three", true))
	require.NoError(t, err, "deleting an LLM frees its slot")
}

func TestCreateLLMUnlimitedOrUnset(t *testing.T) {
	_, _, service := setupManagementTestDB(t)
	for i := 0; i < 3; i++ {
		_, err := service.CreateLLM(llmRequest(fmt.Sprintf("unset-%d", i), true))
		require.NoError(t, err, "no limit set (Community Edition)")
	}
	service.SetLLMLimit(fixedLimit(0, true))
	_, err := service.CreateLLM(llmRequest("unlimited", true))
	require.NoError(t, err)
}

// A licence without max_llms (0) allows none; after a downgrade below the
// current count the existing LLMs stay and new ones are refused.
func TestCreateLLMZeroAndDowngrade(t *testing.T) {
	db, _, service := setupManagementTestDB(t)
	service.SetLLMLimit(fixedLimit(0, false))
	_, err := service.CreateLLM(llmRequest("none", true))
	assert.True(t, IsLLMLimitError(err), "got %v", err)
	assert.Contains(t, err.Error(), "limit of 0 LLMs.")

	service.SetLLMLimit(fixedLimit(0, true))
	for i := 0; i < 3; i++ {
		_, err := service.CreateLLM(llmRequest(fmt.Sprintf("d-%d", i), true))
		require.NoError(t, err)
	}
	service.SetLLMLimit(fixedLimit(1, false))
	_, err = service.CreateLLM(llmRequest("more", true))
	assert.True(t, IsLLMLimitError(err))
	assert.Contains(t, err.Error(), "limit of 1 LLM.")
	n, err := countLLMs(db)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n, "nothing is revoked")
}

// parallelCreatesStayWithinLimit runs attempts LLM creates in parallel,
// alternating between two gateways' services on their own connections to one
// database, and checks that exactly max succeed and the rest are refused.
func parallelCreatesStayWithinLimit(t *testing.T, a, b *gorm.DB) {
	t.Helper()
	const max, attempts = 3, 16
	crypto := NewCryptoService("12345678901234567890123456789012")
	gateways := []*ManagementService{
		NewManagementService(a, database.NewRepository(a), crypto).(*ManagementService),
		NewManagementService(b, database.NewRepository(b), crypto).(*ManagementService),
	}
	for _, g := range gateways {
		g.SetLLMLimit(fixedLimit(max, false))
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var ok, refused int
	var others []error
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := gateways[i%2].CreateLLM(llmRequest(fmt.Sprintf("parallel-%d", i), true))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case IsLLMLimitError(err):
				refused++
			default:
				others = append(others, err)
			}
		}(i)
	}
	wg.Wait()
	require.Empty(t, others)
	assert.Equal(t, max, ok)
	assert.Equal(t, attempts-max, refused)
	n, err := countLLMs(a)
	require.NoError(t, err)
	assert.Equal(t, int64(max), n)
}

// Two gateways on one SQLite file, opened as the gateway opens it.
func TestParallelLLMCreatesWithinLimitSQLiteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.db")
	open := func() *gorm.DB {
		db, err := database.Connect(database.DatabaseConfig{Type: "sqlite", DSN: path, MaxOpenConns: 4, MaxIdleConns: 4, LogLevel: "silent"})
		require.NoError(t, err)
		t.Cleanup(func() { database.Close(db) })
		return db
	}
	a := open()
	require.NoError(t, database.Migrate(a))
	parallelCreatesStayWithinLimit(t, a, open())
}

// Two gateways sharing a Postgres database (MGW_TEST_POSTGRES_DSN).
func TestParallelLLMCreatesWithinLimitPostgres(t *testing.T) {
	a := openPostgresSchema(t)
	parallelCreatesStayWithinLimit(t, a, a.Session(&gorm.Session{NewDB: true}))
}
