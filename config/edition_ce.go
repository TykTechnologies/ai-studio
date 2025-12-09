//go:build !enterprise

package config

// IsEnterprise returns false for Community Edition builds
func IsEnterprise() bool {
	return false
}
