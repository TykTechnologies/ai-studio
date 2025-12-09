package containers

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// SyslogContainer wraps a syslog server container for integration testing.
// It provides both TCP and UDP syslog endpoints for testing audit logging.
type SyslogContainer struct {
	testcontainers.Container
	host     string
	udpPort  string
	tcpPort  string
	logPath  string
	hostname string
}

// SyslogConfig holds configuration for creating a Syslog container.
type SyslogConfig struct {
	// LogPath is the path inside the container where logs are written.
	// Defaults to "/var/log/messages".
	LogPath string
}

// DefaultSyslogConfig returns a default Syslog container configuration.
func DefaultSyslogConfig() *SyslogConfig {
	return &SyslogConfig{
		LogPath: "/var/log/messages",
	}
}

// NewSyslogContainer creates and starts a syslog server container.
// The container accepts both TCP and UDP connections on port 514.
func NewSyslogContainer(ctx context.Context, cfg *SyslogConfig) (*SyslogContainer, error) {
	if cfg == nil {
		cfg = DefaultSyslogConfig()
	}

	if cfg.LogPath == "" {
		cfg.LogPath = "/var/log/messages"
	}

	// Use Alpine with rsyslog, configured to write to /var/log/messages
	// The rsyslog appliance image has complex paths, so we use plain Alpine
	req := testcontainers.ContainerRequest{
		Image:        "alpine:3.19",
		ExposedPorts: []string{"514/udp", "514/tcp"},
		// Install and configure rsyslog to listen on TCP/UDP and write to /var/log/messages
		Cmd: []string{
			"sh", "-c",
			`apk add --no-cache rsyslog && \
			 mkdir -p /var/log && \
			 touch /var/log/messages && \
			 chmod 644 /var/log/messages && \
			 cat > /etc/rsyslog.conf << 'EOF'
module(load="imudp")
input(type="imudp" port="514")
module(load="imtcp")
input(type="imtcp" port="514")
*.* /var/log/messages
EOF
			 rsyslogd -n`,
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("514/tcp").WithStartupTimeout(60*time.Second),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start syslog container: %w", err)
	}

	// Give rsyslog a moment to fully initialize
	time.Sleep(500 * time.Millisecond)

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	udpPort, err := container.MappedPort(ctx, "514/udp")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get UDP mapped port: %w", err)
	}

	tcpPort, err := container.MappedPort(ctx, "514/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get TCP mapped port: %w", err)
	}

	return &SyslogContainer{
		Container: container,
		host:      host,
		udpPort:   udpPort.Port(),
		tcpPort:   tcpPort.Port(),
		logPath:   cfg.LogPath,
		hostname:  "",
	}, nil
}

// Host returns the container's host address.
func (s *SyslogContainer) Host() string {
	return s.host
}

// UDPPort returns the mapped UDP port (as a string).
func (s *SyslogContainer) UDPPort() string {
	return s.udpPort
}

// TCPPort returns the mapped TCP port (as a string).
func (s *SyslogContainer) TCPPort() string {
	return s.tcpPort
}

// UDPAddr returns the UDP address in "host:port" format.
func (s *SyslogContainer) UDPAddr() string {
	return fmt.Sprintf("%s:%s", s.host, s.udpPort)
}

// TCPAddr returns the TCP address in "host:port" format.
func (s *SyslogContainer) TCPAddr() string {
	return fmt.Sprintf("%s:%s", s.host, s.tcpPort)
}

// TCPAddrWithScheme returns the TCP address with "tcp://" prefix.
func (s *SyslogContainer) TCPAddrWithScheme() string {
	return fmt.Sprintf("tcp://%s:%s", s.host, s.tcpPort)
}

// UDPAddrWithScheme returns the UDP address with "udp://" prefix.
func (s *SyslogContainer) UDPAddrWithScheme() string {
	return fmt.Sprintf("udp://%s:%s", s.host, s.udpPort)
}

// Close terminates the syslog container and releases resources.
func (s *SyslogContainer) Close(ctx context.Context) error {
	if s.Container == nil {
		return nil
	}
	return s.Container.Terminate(ctx)
}

// ReadLogs reads and returns all syslog messages from the container.
func (s *SyslogContainer) ReadLogs(ctx context.Context) (string, error) {
	cmd := []string{"cat", s.logPath}

	exitCode, reader, err := s.Container.Exec(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("cat returned exit code %d", exitCode)
	}

	output, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read output: %w", err)
	}

	return string(output), nil
}

// ClearLogs clears all syslog messages from the container.
func (s *SyslogContainer) ClearLogs(ctx context.Context) error {
	cmd := []string{"sh", "-c", fmt.Sprintf("echo '' > %s", s.logPath)}

	exitCode, _, err := s.Container.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to clear logs: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("clear logs returned exit code %d", exitCode)
	}
	return nil
}

// ContainsLog checks if the syslog contains a specific message substring.
func (s *SyslogContainer) ContainsLog(ctx context.Context, substring string) (bool, error) {
	logs, err := s.ReadLogs(ctx)
	if err != nil {
		return false, err
	}

	return len(logs) > 0 && contains(logs, substring), nil
}

// contains is a simple substring check helper.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
