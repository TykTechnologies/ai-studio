package startup

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/nats-io/nats.go"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestConnectivity performs startup connectivity tests for all configured backends
func TestConnectivity(cfg *config.AppConf) error {
	// Skip connectivity tests in CI/test environments to avoid configuration issues
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("SKIP_CONNECTIVITY_TESTS") != "" {
		logger.Info("Skipping connectivity tests in CI/test environment")
		return nil
	}

	logger.Info("Starting connectivity tests...")

	// Test database connectivity
	if err := testDatabaseConnectivity(cfg); err != nil {
		return fmt.Errorf("database connectivity test failed: %w", err)
	}

	// Test queue connectivity (if configured)
	if err := testQueueConnectivity(cfg.QueueConfig); err != nil {
		return fmt.Errorf("queue connectivity test failed: %w", err)
	}

	logger.Info("✅ All connectivity tests passed")
	return nil
}

// testDatabaseConnectivity tests connection to the configured database
func testDatabaseConnectivity(cfg *config.AppConf) error {
	logger.Infof("Testing database connectivity (type: %s)...", cfg.DatabaseType)

	var dialector gorm.Dialector
	switch cfg.DatabaseType {
	case "sqlite":
		dialector = sqlite.Open(cfg.DatabaseURL)
		logger.Infof("Testing SQLite database: %s", cfg.DatabaseURL)
	case "postgres":
		dialector = postgres.Open(cfg.DatabaseURL)
		logger.Infof("Testing PostgreSQL database")
	default:
		return fmt.Errorf("unsupported database type: %s", cfg.DatabaseType)
	}

	// Create a context with timeout for the database connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Open database connection with timeout
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Get the underlying SQL DB to test connectivity
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}
	defer sqlDB.Close()

	// Test with context timeout
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Infof("✅ Database connectivity test passed (%s)", cfg.DatabaseType)
	return nil
}

// testQueueConnectivity tests connection to the configured message queue
func testQueueConnectivity(queueConfig config.QueueConfig) error {
	logger.Infof("Testing queue connectivity (type: %s)...", queueConfig.Type)

	switch queueConfig.Type {
	case "inmemory":
		// In-memory queue doesn't require external connectivity
		logger.Info("✅ In-memory queue connectivity test passed (no external dependencies)")
		return nil

	case "nats":
		return testNATSConnectivity(queueConfig.NATS)

	case "postgres":
		return testPostgreSQLQueueConnectivity(queueConfig.PostgreSQL)

	default:
		return fmt.Errorf("unsupported queue type: %s", queueConfig.Type)
	}
}

// testNATSConnectivity tests connection to NATS server and JetStream
func testNATSConnectivity(natsConfig config.NATSConfig) error {
	logger.Infof("Testing NATS connectivity to: %s", natsConfig.URL)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test basic TCP connectivity first
	if err := testNATSTCPConnection(ctx, natsConfig.URL); err != nil {
		return fmt.Errorf("NATS TCP connectivity failed: %w", err)
	}

	// Configure NATS connection options
	opts := []nats.Option{
		nats.Timeout(10 * time.Second),
		nats.ReconnectWait(1 * time.Second),
		nats.MaxReconnects(1), // Limited for startup test
		nats.ErrorHandler(func(conn *nats.Conn, s *nats.Subscription, err error) {
			logger.Infof("NATS startup test error: %v", err)
		}),
	}

	// Add authentication options
	if err := addNATSAuthOptions(&opts, natsConfig); err != nil {
		return fmt.Errorf("failed to configure NATS authentication: %w", err)
	}

	// Test NATS connection
	conn, err := nats.Connect(natsConfig.URL, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	defer conn.Close()

	// Verify connection is established
	if !conn.IsConnected() {
		return fmt.Errorf("NATS connection not established")
	}

	// Test JetStream availability
	js, err := conn.JetStream()
	if err != nil {
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}

	// Test JetStream with a simple operation (get account info)
	info, err := js.AccountInfo()
	if err != nil {
		return fmt.Errorf("failed to get JetStream account info: %w", err)
	}

	logger.Infof("✅ NATS connectivity test passed (JetStream enabled: %t, Memory: %d, Storage: %d)", 
		true, info.Memory, info.Store)

	return nil
}

// testNATSTCPConnection tests basic TCP connectivity to NATS server
func testNATSTCPConnection(ctx context.Context, natsURL string) error {
	// Parse NATS URL to get host:port
	host, port, err := parseNATSURL(natsURL)
	if err != nil {
		return fmt.Errorf("failed to parse NATS URL: %w", err)
	}

	// Test TCP connection
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%s", host, port))
	if err != nil {
		return fmt.Errorf("failed to establish TCP connection to %s:%s: %w", host, port, err)
	}
	conn.Close()

	return nil
}

// parseNATSURL extracts host and port from NATS URL
func parseNATSURL(natsURL string) (host, port string, err error) {
	// Handle common NATS URL formats
	// nats://localhost:4222
	// nats://user:pass@localhost:4222
	// localhost:4222
	
	// Simple parsing - remove protocol if present
	url := natsURL
	if len(url) > 7 && url[:7] == "nats://" {
		url = url[7:]
	}

	// Remove user:pass@ if present
	if atIndex := len(url) - 1; atIndex >= 0 {
		for i := len(url) - 1; i >= 0; i-- {
			if url[i] == '@' {
				url = url[i+1:]
				break
			}
		}
	}

	// Split host:port
	host = "localhost"
	port = "4222"
	
	colonIndex := -1
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == ':' {
			colonIndex = i
			break
		}
	}
	
	if colonIndex > 0 {
		host = url[:colonIndex]
		port = url[colonIndex+1:]
	} else if len(url) > 0 && colonIndex == -1 {
		// No port specified, use the URL as host
		host = url
	}

	if host == "" {
		host = "localhost"
	}

	return host, port, nil
}

// addNATSAuthOptions configures NATS authentication options
// This is a copy of the function from chat_session/queue_nats.go to avoid circular imports
func addNATSAuthOptions(opts *[]nats.Option, config config.NATSConfig) error {
	// Handle credentials file authentication (JWT/NKeys)
	if config.CredentialsFile != "" {
		*opts = append(*opts, nats.UserCredentials(config.CredentialsFile))
		logger.Infof("NATS authentication configured with credentials file: %s", config.CredentialsFile)
	}
	
	// Handle NKey file authentication
	if config.NKeyFile != "" {
		*opts = append(*opts, nats.UserCredentials(config.NKeyFile))
		logger.Infof("NATS authentication configured with NKey file: %s", config.NKeyFile)
	}
	
	// Handle basic username/password authentication
	if config.Username != "" && config.Password != "" {
		*opts = append(*opts, nats.UserInfo(config.Username, config.Password))
		logger.Infof("NATS authentication configured with username: %s", config.Username)
	}
	
	// Handle token-based authentication
	if config.Token != "" {
		*opts = append(*opts, nats.Token(config.Token))
		logger.Infof("NATS authentication configured with token")
	}
	
	// Handle TLS configuration
	if config.TLSEnabled {
		*opts = append(*opts, nats.Secure())
		logger.Infof("NATS TLS connection enabled")
	}
	
	return nil
}

// testPostgreSQLQueueConnectivity tests PostgreSQL connectivity for queue functionality
func testPostgreSQLQueueConnectivity(pgConfig config.PostgreSQLQueueConfig) error {
	logger.Info("Testing PostgreSQL queue connectivity...")

	// PostgreSQL queues require DATABASE_URL environment variable
	databaseURL := config.Get("").DatabaseURL
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required for PostgreSQL queues")
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test database connection for PostgreSQL queues
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL for queue: %w", err)
	}

	// Get the underlying SQL DB to test connectivity  
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get PostgreSQL database instance: %w", err)
	}
	defer sqlDB.Close()

	// Test database connectivity
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL database: %w", err)
	}

	// Test PostgreSQL LISTEN/NOTIFY functionality (basic test)
	// We'll test if we can execute a simple NOTIFY command
	_, err = sqlDB.ExecContext(ctx, "SELECT pg_notify('test_connectivity_check', 'startup_test')")
	if err != nil {
		return fmt.Errorf("PostgreSQL LISTEN/NOTIFY test failed: %w", err)
	}

	logger.Info("✅ PostgreSQL queue connectivity test passed (LISTEN/NOTIFY available)")
	return nil
}