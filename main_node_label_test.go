//go:build !enterprise

package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nodeLabels reads the labels in the child's cluster registry.
func nodeLabels(t *testing.T, dbPath string) []string {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro&_busy_timeout=5000")
	require.NoError(t, err)
	defer db.Close()
	rows, err := db.Query("SELECT label FROM cluster_nodes")
	require.NoError(t, err)
	defer rows.Close()
	var labels []string
	for rows.Next() {
		var label string
		require.NoError(t, rows.Scan(&label))
		labels = append(labels, label)
	}
	require.NoError(t, rows.Err())
	return labels
}

// CLUSTER_NODE_LABEL names the replica in the cluster registry (what the
// cluster status and the Edge Gateways page show); unset keeps "studio".
func TestClusterNodeLabelSetting(t *testing.T) {
	for _, tc := range []struct{ name, value, want string }{
		{"set", " site-a ", "site-a"},
		{"unset", "", "studio"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := freePort(t)
			dbPath := filepath.Join(t.TempDir(), "studio.db")
			extra := map[string]string{"SERVER_PORT": strconv.Itoa(port), "DATABASE_URL": dbPath}
			if tc.value != "" {
				extra["CLUSTER_NODE_LABEL"] = tc.value
			}
			cmd, done := startChild(t, extra)

			require.Eventually(t, func() bool {
				resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
				if err != nil {
					return false
				}
				resp.Body.Close()
				return resp.StatusCode == http.StatusOK
			}, 2*time.Minute, 100*time.Millisecond, "Studio serves")

			assert.Equal(t, []string{tc.want}, nodeLabels(t, dbPath))

			require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
			assert.Equal(t, 0, exitCode(t, done, time.Minute))
		})
	}
}

// A label the registry refuses stops the process before it serves.
func TestInvalidClusterNodeLabelExitsNonZero(t *testing.T) {
	for _, bad := range []string{"<site-a>", strings.Repeat("x", 65)} {
		_, done := startChild(t, map[string]string{"CLUSTER_NODE_LABEL": bad})
		assert.Equal(t, 1, exitCode(t, done, 2*time.Minute), "%q", bad)
	}
}
