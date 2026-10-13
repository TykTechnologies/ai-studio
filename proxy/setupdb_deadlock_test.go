package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// setupDB's single connection must not deadlock when a transaction runs a
// statement that another goroutine, waiting for that connection, is about to
// run too. With gorm's PrepareStmt cache the transaction waited for the other
// goroutine to finish preparing the statement, and that goroutine waited for
// the connection the transaction held: TestEmbeddedBudget_UnpacedBurst-
// OvershootIsBounded hung until the package timed out (the budget analysis
// and the chat-record analysis both look up the model price, one inside a
// transaction).
func TestSetupDBTransactionDoesNotWaitOnAnotherGoroutinesPrepare(t *testing.T) {
	db := setupDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)

	const query = "SELECT count(*) FROM llms WHERE id > ?"

	tx := db.Begin()
	require.NoError(t, tx.Error)
	var one int
	require.NoError(t, tx.Raw("SELECT 1").Scan(&one).Error) // the transaction holds the connection

	other := make(chan error, 1)
	go func() {
		var n int64
		other <- db.Raw(query, 0).Scan(&n).Error
	}()
	// The other goroutine is now waiting for the connection.
	require.Eventually(t, func() bool { return sqlDB.Stats().WaitCount > 0 }, 5*time.Second, 5*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		var n int64
		if err := tx.Raw(query, 0).Scan(&n).Error; err != nil {
			done <- err
			return
		}
		done <- tx.Commit().Error
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the transaction's statement waited on the other goroutine, which waits for the transaction's connection")
	}
	select {
	case err := <-other:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the other goroutine never got the connection back")
	}
}
