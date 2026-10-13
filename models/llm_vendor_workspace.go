package models

import (
	"fmt"
	"regexp"
)

// vendorWorkspaceIDPattern bounds what may go into the workspace header:
// vendor workspace IDs are short tokens (Anthropic: wrkspc_...).
var vendorWorkspaceIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ValidateVendorWorkspaceID checks an LLM's VendorWorkspaceID. Empty is
// valid (no pin).
func ValidateVendorWorkspaceID(id string) error {
	if id == "" || vendorWorkspaceIDPattern.MatchString(id) {
		return nil
	}
	return fmt.Errorf("vendor_workspace_id must be 1-128 letters, digits, '_' or '-'")
}
