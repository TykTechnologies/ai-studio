package grpc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"google.golang.org/protobuf/proto"
)

// ComputeSnapshotChecksum generates a deterministic SHA-256 checksum for a ConfigurationSnapshot.
// It excludes volatile fields (version, snapshot_time, checksum, timestamps, usage stats) to ensure
// the checksum only changes when actual configuration data changes.
//
// The checksum is computed from the protobuf-serialized bytes of the snapshot with
// deterministic marshaling enabled to ensure consistent byte order for maps and repeated fields.
func ComputeSnapshotChecksum(snapshot *pb.ConfigurationSnapshot) (string, error) {
	if snapshot == nil {
		return "", fmt.Errorf("snapshot is nil")
	}

	// Create a copy with volatile fields cleared
	checksumSnapshot := proto.Clone(snapshot).(*pb.ConfigurationSnapshot)
	checksumSnapshot.Version = ""       // Exclude version (timestamp)
	checksumSnapshot.SnapshotTime = nil // Exclude timestamp
	checksumSnapshot.Checksum = ""      // Exclude checksum itself

	// Clear volatile fields from nested LLMConfig messages
	for _, llm := range checksumSnapshot.Llms {
		llm.CreatedAt = nil
		llm.UpdatedAt = nil
	}

	// Clear volatile fields from nested AppConfig messages
	for _, app := range checksumSnapshot.Apps {
		app.CreatedAt = nil
		app.UpdatedAt = nil
		app.CurrentPeriodUsage = 0 // Budget usage changes constantly
	}

	// Clear volatile fields from nested FilterConfig messages
	for _, filter := range checksumSnapshot.Filters {
		filter.CreatedAt = nil
		filter.UpdatedAt = nil
	}

	// Clear volatile fields from nested PluginConfig messages
	for _, plugin := range checksumSnapshot.Plugins {
		plugin.CreatedAt = nil
		plugin.UpdatedAt = nil
	}

	// Clear volatile fields from nested ModelPriceConfig messages
	for _, price := range checksumSnapshot.ModelPrices {
		price.CreatedAt = nil
		price.UpdatedAt = nil
	}

	// Clear volatile fields from nested ModelRouterConfig messages
	for _, router := range checksumSnapshot.ModelRouters {
		router.CreatedAt = nil
		router.UpdatedAt = nil
	}

	// Use deterministic marshaling to ensure consistent byte order for maps and repeated fields
	opts := proto.MarshalOptions{Deterministic: true}
	data, err := opts.Marshal(checksumSnapshot)
	if err != nil {
		return "", fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	// Compute SHA-256 hash
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// VerifySnapshotChecksum verifies that a snapshot's checksum matches the computed checksum.
// Returns true if the checksums match, false otherwise.
func VerifySnapshotChecksum(snapshot *pb.ConfigurationSnapshot) (bool, error) {
	if snapshot == nil {
		return false, fmt.Errorf("snapshot is nil")
	}

	if snapshot.Checksum == "" {
		return false, fmt.Errorf("snapshot has no checksum")
	}

	computed, err := ComputeSnapshotChecksum(snapshot)
	if err != nil {
		return false, err
	}

	return computed == snapshot.Checksum, nil
}
