package cluster

import (
	"strings"
	"syscall"
)

func bootID() string {
	id, err := syscall.Sysctl("kern.bootsessionuuid")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(id)
}

// macOS has no pid namespaces: pids are unique on the machine.
func pidNamespace() string { return "host" }
