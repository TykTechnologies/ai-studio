package main

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/grpc"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	lkgHelperEnv = "MGW_TEST_LKG_HELPER"
	lkgTestKey   = "0123456789abcdef0123456789abcdef"
)

func lkgTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	return db
}

// TestMustStartEdgeClientHelper is not a test: it is the body of the child
// process TestEdgeStartExitCodes runs. It starts an edge the way main does,
// with no control plane to reach, and prints "STARTED" if the process is
// still alive afterwards.
func TestMustStartEdgeClientHelper(t *testing.T) {
	if os.Getenv(lkgHelperEnv) != "1" {
		t.Skip("runs only as the child process of TestEdgeStartExitCodes")
	}
	cfg := &config.Config{
		Security: config.SecurityConfig{EncryptionKey: os.Getenv("MGW_TEST_LKG_KEY")},
		HubSpoke: config.HubSpokeConfig{
			Mode:                   "edge",
			EdgeID:                 "exit-edge",
			EdgeNamespace:          os.Getenv("MGW_TEST_LKG_NAMESPACE"),
			ControlEndpoint:        os.Getenv("MGW_TEST_LKG_ENDPOINT"),
			AllowInsecure:          true,
			ReconnectInterval:      10 * time.Millisecond,
			HeartbeatInterval:      time.Hour,
			StartFromLastKnownGood: os.Getenv("MGW_TEST_LKG_ENABLED") == "true",
			StartupControlTimeout:  50 * time.Millisecond,
			LastKnownGoodMaxAge:    72 * time.Hour,
		},
	}
	if v := os.Getenv("MGW_TEST_LKG_MAX_AGE"); v != "" {
		d, err := time.ParseDuration(v)
		require.NoError(t, err)
		cfg.HubSpoke.LastKnownGoodMaxAge = d
	}
	db := lkgTestDB(t, os.Getenv("MGW_TEST_LKG_DB"))
	client := grpc.NewSimpleEdgeClient(cfg, "test", "hash", "time")
	lkg := mustStartEdgeClient(client, db) // exits the process when the edge cannot start
	if lkg != nil {
		os.Stdout.WriteString("STARTED last_known_good " + lkg.Checksum + "\n")
	} else {
		os.Stdout.WriteString("STARTED control\n")
	}
	_ = client.Stop()
}

// An edge that cannot start exits non-zero and says why; one that can start
// from last-known-good does not exit. The start-up runs in a child process,
// through the function main calls, so the exit is the real one.
func TestEdgeStartExitCodes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs child processes")
	}
	dir := t.TempDir()

	// An address nothing listens on: no control plane.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	endpoint := lis.Addr().String()
	lis.Close()

	// A database with a snapshot stored for namespace "emea".
	seeded := filepath.Join(dir, "seeded.db")
	db := lkgTestDB(t, seeded)
	require.NoError(t, services.PersistAppliedSnapshots(db, lkgTestKey))
	now := time.Now()
	require.NoError(t, services.NewEdgeSyncService(db, "emea").SyncConfiguration(&pb.ConfigurationSnapshot{
		Version: "v1", Checksum: "stored-checksum", SnapshotTime: timestamppb.New(now),
		Llms: []*pb.LLMConfig{{Id: 1, Name: "LLM", Slug: "llm", Vendor: "openai", IsActive: true,
			CreatedAt: timestamppb.New(now.Add(-time.Hour)), UpdatedAt: timestamppb.New(now.Add(-time.Hour))}},
	}))
	require.NoError(t, database.Close(db))

	run := func(env map[string]string) (exitCode int, output string) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestMustStartEdgeClientHelper$", "-test.v")
		cmd.Env = append(os.Environ(), lkgHelperEnv+"=1", "MGW_TEST_LKG_ENDPOINT="+endpoint)
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), string(out)
		}
		require.NoError(t, err, "%s", out)
		return 0, string(out)
	}
	base := func(extra map[string]string) map[string]string {
		env := map[string]string{
			"MGW_TEST_LKG_ENABLED":   "true",
			"MGW_TEST_LKG_DB":        seeded,
			"MGW_TEST_LKG_KEY":       lkgTestKey,
			"MGW_TEST_LKG_NAMESPACE": "emea",
		}
		for k, v := range extra {
			env[k] = v
		}
		return env
	}
	const fatal = "edge cannot start without configuration"

	t.Run("starts from last-known-good", func(t *testing.T) {
		code, out := run(base(nil))
		assert.Equal(t, 0, code, out)
		assert.Contains(t, out, "STARTED last_known_good stored-checksum")
		assert.Contains(t, out, "serving the last-known-good configuration")
	})

	for name, tc := range map[string]struct {
		env    map[string]string
		reason string
	}{
		"the option is off (as today)":     {map[string]string{"MGW_TEST_LKG_ENABLED": "false"}, "failed to"},
		"no stored snapshot":               {map[string]string{"MGW_TEST_LKG_DB": filepath.Join(dir, "empty.db")}, "no configuration snapshot is stored"},
		"another namespace":                {map[string]string{"MGW_TEST_LKG_NAMESPACE": "apac"}, `is for namespace \"emea\"`},
		"another encryption key":           {map[string]string{"MGW_TEST_LKG_KEY": "fedcba9876543210fedcba9876543210"}, "does not decrypt with this ENCRYPTION_KEY"},
		"too long without a control plane": {map[string]string{"MGW_TEST_LKG_MAX_AGE": "1ns"}, "longer than EDGE_LAST_KNOWN_GOOD_MAX_AGE"},
	} {
		t.Run(name, func(t *testing.T) {
			code, out := run(base(tc.env))
			assert.Equal(t, 1, code, "the edge must exit non-zero: %s", out)
			assert.NotContains(t, out, "STARTED")
			assert.Contains(t, out, fatal)
			assert.Contains(t, out, tc.reason, "the reason is logged")
		})
	}
}
