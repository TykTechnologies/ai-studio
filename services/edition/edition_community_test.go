//go:build !enterprise

package edition

import "testing"

func TestCheckRegisteredPassesInCommunityBuilds(t *testing.T) {
	if err := CheckRegistered(); err != nil {
		t.Fatalf("community build reported missing enterprise features: %v", err)
	}
}
