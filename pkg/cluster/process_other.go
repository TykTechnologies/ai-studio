//go:build !linux && !darwin

package cluster

// Elsewhere this process cannot tell a dead predecessor from a live
// replica: an empty boot ID turns early take-over off.
func bootID() string { return "" }

func pidNamespace() string { return "" }

func processAlive(int) bool { return true }
