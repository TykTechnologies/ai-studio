package grpc

import (
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestComputeSnapshotChecksum(t *testing.T) {
	t.Run("nil snapshot returns error", func(t *testing.T) {
		checksum, err := ComputeSnapshotChecksum(nil)
		assert.Error(t, err)
		assert.Empty(t, checksum)
	})

	t.Run("empty snapshot produces valid checksum", func(t *testing.T) {
		snapshot := &pb.ConfigurationSnapshot{}
		checksum, err := ComputeSnapshotChecksum(snapshot)
		require.NoError(t, err)
		assert.NotEmpty(t, checksum)
		assert.Len(t, checksum, 64) // SHA-256 hex string is 64 characters
	})

	t.Run("same data produces same checksum", func(t *testing.T) {
		snapshot1 := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm", Vendor: "openai"},
			},
			Apps: []*pb.AppConfig{
				{Id: 1, Name: "test-app"},
			},
		}
		snapshot2 := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm", Vendor: "openai"},
			},
			Apps: []*pb.AppConfig{
				{Id: 1, Name: "test-app"},
			},
		}

		checksum1, err := ComputeSnapshotChecksum(snapshot1)
		require.NoError(t, err)

		checksum2, err := ComputeSnapshotChecksum(snapshot2)
		require.NoError(t, err)

		assert.Equal(t, checksum1, checksum2, "Same data should produce same checksum")
	})

	t.Run("different data produces different checksum", func(t *testing.T) {
		snapshot1 := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm-1", Vendor: "openai"},
			},
		}
		snapshot2 := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm-2", Vendor: "openai"},
			},
		}

		checksum1, err := ComputeSnapshotChecksum(snapshot1)
		require.NoError(t, err)

		checksum2, err := ComputeSnapshotChecksum(snapshot2)
		require.NoError(t, err)

		assert.NotEqual(t, checksum1, checksum2, "Different data should produce different checksum")
	})

	t.Run("version field is excluded from checksum", func(t *testing.T) {
		snapshot1 := &pb.ConfigurationSnapshot{
			Version: "1234567890",
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm"},
			},
		}
		snapshot2 := &pb.ConfigurationSnapshot{
			Version: "9876543210",
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm"},
			},
		}

		checksum1, err := ComputeSnapshotChecksum(snapshot1)
		require.NoError(t, err)

		checksum2, err := ComputeSnapshotChecksum(snapshot2)
		require.NoError(t, err)

		assert.Equal(t, checksum1, checksum2, "Version should be excluded from checksum")
	})

	t.Run("snapshot_time field is excluded from checksum", func(t *testing.T) {
		snapshot1 := &pb.ConfigurationSnapshot{
			SnapshotTime: timestamppb.Now(),
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm"},
			},
		}
		snapshot2 := &pb.ConfigurationSnapshot{
			SnapshotTime: timestamppb.Now(),
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm"},
			},
		}

		checksum1, err := ComputeSnapshotChecksum(snapshot1)
		require.NoError(t, err)

		checksum2, err := ComputeSnapshotChecksum(snapshot2)
		require.NoError(t, err)

		assert.Equal(t, checksum1, checksum2, "SnapshotTime should be excluded from checksum")
	})

	t.Run("checksum field is excluded from computation", func(t *testing.T) {
		snapshot1 := &pb.ConfigurationSnapshot{
			Checksum: "existing-checksum-1",
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm"},
			},
		}
		snapshot2 := &pb.ConfigurationSnapshot{
			Checksum: "existing-checksum-2",
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm"},
			},
		}

		checksum1, err := ComputeSnapshotChecksum(snapshot1)
		require.NoError(t, err)

		checksum2, err := ComputeSnapshotChecksum(snapshot2)
		require.NoError(t, err)

		assert.Equal(t, checksum1, checksum2, "Existing checksum field should be excluded from computation")
	})

	t.Run("nested timestamps are excluded from checksum", func(t *testing.T) {
		snapshot1 := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm", CreatedAt: timestamppb.Now(), UpdatedAt: timestamppb.Now()},
			},
			Apps: []*pb.AppConfig{
				{Id: 1, Name: "test-app", CreatedAt: timestamppb.Now(), UpdatedAt: timestamppb.Now()},
			},
		}
		snapshot2 := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm", CreatedAt: timestamppb.Now(), UpdatedAt: timestamppb.Now()},
			},
			Apps: []*pb.AppConfig{
				{Id: 1, Name: "test-app", CreatedAt: timestamppb.Now(), UpdatedAt: timestamppb.Now()},
			},
		}

		checksum1, err := ComputeSnapshotChecksum(snapshot1)
		require.NoError(t, err)

		checksum2, err := ComputeSnapshotChecksum(snapshot2)
		require.NoError(t, err)

		assert.Equal(t, checksum1, checksum2, "Nested timestamps should be excluded from checksum")
	})

	t.Run("apps are excluded entirely from checksum for pull-on-miss sync", func(t *testing.T) {
		// Apps are excluded from checksum because they can be synced via pull-on-miss
		// which is out-of-band from the normal snapshot sync mechanism
		snapshot1 := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm", Vendor: "openai"},
			},
			Apps: []*pb.AppConfig{
				{Id: 1, Name: "app-1"},
			},
		}
		snapshot2 := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm", Vendor: "openai"},
			},
			Apps: []*pb.AppConfig{
				{Id: 1, Name: "app-1"},
				{Id: 2, Name: "app-2"},
				{Id: 3, Name: "app-3"},
			},
		}

		checksum1, err := ComputeSnapshotChecksum(snapshot1)
		require.NoError(t, err)

		checksum2, err := ComputeSnapshotChecksum(snapshot2)
		require.NoError(t, err)

		assert.Equal(t, checksum1, checksum2, "Apps should be excluded entirely from checksum")
	})

	t.Run("apps excluded but llm changes still affect checksum", func(t *testing.T) {
		// Verify that while Apps are excluded, LLM changes still affect the checksum
		snapshot1 := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "llm-1", Vendor: "openai"},
			},
			Apps: []*pb.AppConfig{
				{Id: 1, Name: "app-1"},
			},
		}
		snapshot2 := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "llm-1", Vendor: "openai"},
				{Id: 2, Name: "llm-2", Vendor: "anthropic"},
			},
			Apps: []*pb.AppConfig{
				{Id: 1, Name: "app-1"},
			},
		}

		checksum1, err := ComputeSnapshotChecksum(snapshot1)
		require.NoError(t, err)

		checksum2, err := ComputeSnapshotChecksum(snapshot2)
		require.NoError(t, err)

		assert.NotEqual(t, checksum1, checksum2, "LLM changes should still affect checksum even though Apps are excluded")
	})

	t.Run("complex snapshot produces consistent checksum", func(t *testing.T) {
		snapshot := &pb.ConfigurationSnapshot{
			Version:       "1234567890",
			EdgeNamespace: "production",
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "gpt-4", Vendor: "openai", IsActive: true},
				{Id: 2, Name: "claude", Vendor: "anthropic", IsActive: true},
			},
			Apps: []*pb.AppConfig{
				{Id: 1, Name: "app-1", MonthlyBudget: 100.0},
				{Id: 2, Name: "app-2", MonthlyBudget: 200.0},
			},
			Filters: []*pb.FilterConfig{
				{Id: 1, Name: "filter-1", IsActive: true},
			},
			Plugins: []*pb.PluginConfig{
				{Id: 1, Name: "plugin-1", IsActive: true},
			},
		}

		// Compute checksum multiple times to ensure determinism
		var checksums []string
		for i := 0; i < 5; i++ {
			checksum, err := ComputeSnapshotChecksum(snapshot)
			require.NoError(t, err)
			checksums = append(checksums, checksum)
		}

		// All checksums should be identical
		for i := 1; i < len(checksums); i++ {
			assert.Equal(t, checksums[0], checksums[i], "Checksum should be deterministic")
		}
	})
}

func TestVerifySnapshotChecksum(t *testing.T) {
	t.Run("nil snapshot returns error", func(t *testing.T) {
		valid, err := VerifySnapshotChecksum(nil)
		assert.Error(t, err)
		assert.False(t, valid)
	})

	t.Run("snapshot without checksum returns error", func(t *testing.T) {
		snapshot := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm"},
			},
		}
		valid, err := VerifySnapshotChecksum(snapshot)
		assert.Error(t, err)
		assert.False(t, valid)
	})

	t.Run("valid checksum returns true", func(t *testing.T) {
		snapshot := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm"},
			},
		}

		// Compute and set checksum
		checksum, err := ComputeSnapshotChecksum(snapshot)
		require.NoError(t, err)
		snapshot.Checksum = checksum

		// Verify
		valid, err := VerifySnapshotChecksum(snapshot)
		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("invalid checksum returns false", func(t *testing.T) {
		snapshot := &pb.ConfigurationSnapshot{
			Checksum: "invalid-checksum",
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm"},
			},
		}

		valid, err := VerifySnapshotChecksum(snapshot)
		require.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("modified snapshot fails verification", func(t *testing.T) {
		snapshot := &pb.ConfigurationSnapshot{
			Llms: []*pb.LLMConfig{
				{Id: 1, Name: "test-llm"},
			},
		}

		// Compute and set checksum
		checksum, err := ComputeSnapshotChecksum(snapshot)
		require.NoError(t, err)
		snapshot.Checksum = checksum

		// Modify data
		snapshot.Llms[0].Name = "modified-llm"

		// Verify should fail
		valid, err := VerifySnapshotChecksum(snapshot)
		require.NoError(t, err)
		assert.False(t, valid)
	})
}
