package cluster

import (
	"os"
	"strings"
)

func bootID() string {
	b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// pidNamespace is the pid namespace's identity, "pid:[4026531836]". A
// container restart gets a new one; an inode can be reused only once its
// namespace is gone, and then every process that was in it is gone too.
func pidNamespace() string {
	ns, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return ""
	}
	return ns
}
