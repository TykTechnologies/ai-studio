package chat_session

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
)

// CreateQueueFactory creates appropriate queue factory based on configuration
func CreateQueueFactory(cfg config.QueueConfig) (QueueFactory, error) {
	switch cfg.Type {
	case "nats":
		return createNATSQueueFactory(cfg)
	case "inmemory":
		return NewDefaultQueueFactory(cfg.BufferSize), nil
	default:
		return nil, fmt.Errorf("unsupported queue type: %s", cfg.Type)
	}
}

// CreateQueueFactoryFromConfig creates queue factory from global configuration
func CreateQueueFactoryFromConfig() (QueueFactory, error) {
	cfg := config.Get()
	return CreateQueueFactory(cfg.QueueConfig)
}

// createNATSQueueFactory creates NATS queue factory with proper configuration conversion
func createNATSQueueFactory(cfg config.QueueConfig) (QueueFactory, error) {
	natsConfig := NATSConfig{
		URL:             cfg.NATS.URL,
		StorageType:     cfg.NATS.StorageType,
		RetentionPolicy: cfg.NATS.RetentionPolicy,
		DurableConsumer: cfg.NATS.DurableConsumer,
		MaxDeliver:      cfg.NATS.MaxDeliver,
		MaxRetries:      cfg.NATS.MaxRetries,
		BufferSize:      cfg.BufferSize,
		
		// Authentication configuration
		CredentialsFile: cfg.NATS.CredentialsFile,
		Username:        cfg.NATS.Username,
		Password:        cfg.NATS.Password,
		Token:           cfg.NATS.Token,
		NKeyFile:        cfg.NATS.NKeyFile,
		
		// TLS configuration
		TLSEnabled:      cfg.NATS.TLSEnabled,
		TLSCertFile:     cfg.NATS.TLSCertFile,
		TLSKeyFile:      cfg.NATS.TLSKeyFile,
		TLSCAFile:       cfg.NATS.TLSCAFile,
		TLSSkipVerify:   cfg.NATS.TLSSkipVerify,
	}

	// Parse MaxAge duration string
	if cfg.NATS.MaxAge != "" {
		duration, err := time.ParseDuration(cfg.NATS.MaxAge)
		if err != nil {
			slog.Warn("invalid NATS max age duration, using default", "value", cfg.NATS.MaxAge, "error", err)
			natsConfig.MaxAge = 2 * time.Hour
		} else {
			natsConfig.MaxAge = duration
		}
	} else {
		natsConfig.MaxAge = 2 * time.Hour
	}

	// Parse AckWait duration string
	if cfg.NATS.AckWait != "" {
		duration, err := time.ParseDuration(cfg.NATS.AckWait)
		if err != nil {
			slog.Warn("invalid NATS ack wait duration, using default", "value", cfg.NATS.AckWait, "error", err)
			natsConfig.AckWait = 30 * time.Second
		} else {
			natsConfig.AckWait = duration
		}
	} else {
		natsConfig.AckWait = 30 * time.Second
	}

	// Parse FetchTimeout duration string
	if cfg.NATS.FetchTimeout != "" {
		duration, err := time.ParseDuration(cfg.NATS.FetchTimeout)
		if err != nil {
			slog.Warn("invalid NATS fetch timeout duration, using default", "value", cfg.NATS.FetchTimeout, "error", err)
			natsConfig.FetchTimeout = 5 * time.Second
		} else {
			natsConfig.FetchTimeout = duration
		}
	} else {
		natsConfig.FetchTimeout = 5 * time.Second
	}

	// Parse RetryInterval duration string
	if cfg.NATS.RetryInterval != "" {
		duration, err := time.ParseDuration(cfg.NATS.RetryInterval)
		if err != nil {
			slog.Warn("invalid NATS retry interval duration, using default", "value", cfg.NATS.RetryInterval, "error", err)
			natsConfig.RetryInterval = 1 * time.Second
		} else {
			natsConfig.RetryInterval = duration
		}
	} else {
		natsConfig.RetryInterval = 1 * time.Second
	}

	// Set MaxBytes
	if cfg.NATS.MaxBytes > 0 {
		natsConfig.MaxBytes = cfg.NATS.MaxBytes
	} else {
		natsConfig.MaxBytes = 100 * 1024 * 1024 // 100MB default
	}

	slog.Info("creating NATS queue factory",
		"url", natsConfig.URL,
		"storage", natsConfig.StorageType,
		"retention", natsConfig.RetentionPolicy,
		"max_age", natsConfig.MaxAge,
		"max_bytes", natsConfig.MaxBytes,
		"durable", natsConfig.DurableConsumer,
		"tls_enabled", natsConfig.TLSEnabled,
		"auth_configured", natsConfig.CredentialsFile != "" || natsConfig.Username != "" || natsConfig.Token != "" || natsConfig.NKeyFile != "",
	)

	return NewNATSQueueFactory(natsConfig), nil
}

// CreateDefaultQueueFactory creates the default queue factory based on global configuration
func CreateDefaultQueueFactory() QueueFactory {
	cfg := config.Get()

	factory, err := CreateQueueFactory(cfg.QueueConfig)
	if err != nil {
		slog.Warn("failed to create configured queue factory, using in-memory", "error", err)
		return NewDefaultQueueFactory(cfg.QueueConfig.BufferSize)
	}

	return factory
}

// Helper function to create queue with automatic factory selection
func CreateDefaultQueue(sessionID string) (MessageQueue, error) {
	factory := CreateDefaultQueueFactory()
	return factory.CreateQueue(sessionID, nil)
}
