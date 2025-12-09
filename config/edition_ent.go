//go:build enterprise

package config

// IsEnterprise returns true for Enterprise Edition builds
func IsEnterprise() bool {
	return true
}
