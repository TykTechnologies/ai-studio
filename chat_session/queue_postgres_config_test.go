package chat_session

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "github.com/TykTechnologies/midsommar/v2/config"
)

// A host embedding Studio sets the database URL on the configuration, not
// in the environment: the deferred Postgres queue factory uses it.
func TestDeferredPostgreSQLQueueFactory_UsesConfiguredDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	os.Unsetenv("DATABASE_URL")
	t.Cleanup(appconfig.ResetGlobalConfig)
	f := NewDeferredPostgreSQLQueueFactory(DefaultPostgreSQLConfig())

	appconfig.ResetGlobalConfig()
	_, err := f.CreateQueue("s1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database URL")

	// Unreachable on purpose: what matters is that it tries this URL.
	appconfig.Set(&appconfig.AppConf{DatabaseURL: "postgres://studio@127.0.0.1:1/studio?sslmode=disable&connect_timeout=1"})
	_, err = f.CreateQueue("s1", nil)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "is required", "the configured URL was used")
}
