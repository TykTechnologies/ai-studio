// cmd/microgateway/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/server"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Version information (set by build)
var (
	Version   = "dev"
	BuildHash = "unknown"
	BuildTime = "unknown"
)

func main() {
	// Parse command line flags
	var (
		envFile = flag.String("env", ".env", "Path to environment file")
		migrate = flag.Bool("migrate", false, "Run database migrations and exit")
		version = flag.Bool("version", false, "Show version and exit")
		_       = flag.String("config", "", "Path to config file (optional)")
	)
	flag.Parse()

	// Show version if requested
	if *version {
		fmt.Printf("Microgateway v%s\n", Version)
		fmt.Printf("Build Hash: %s\n", BuildHash)
		fmt.Printf("Build Time: %s\n", BuildTime)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*envFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Setup logging
	setupLogging(cfg.Observability)

	log.Info().
		Str("version", Version).
		Str("build", BuildHash).
		Str("build_time", BuildTime).
		Msg("Starting Microgateway")

	// Connect to database
	dbConfig := database.DatabaseConfig{
		Type:            cfg.Database.Type,
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		AutoMigrate:     cfg.Database.AutoMigrate,
		LogLevel:        cfg.Database.LogLevel,
	}

	db, err := database.Connect(dbConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer func() {
		if err := database.Close(db); err != nil {
			log.Error().Err(err).Msg("Failed to close database connection")
		}
	}()

	// Run migrations if requested or auto-migrate is enabled
	if *migrate || cfg.Database.AutoMigrate {
		log.Info().Msg("Running database migrations...")
		if err := database.Migrate(db); err != nil {
			log.Fatal().Err(err).Msg("Failed to run migrations")
		}
		if *migrate {
			log.Info().Msg("Migrations completed successfully")
			os.Exit(0)
		}
	}

	// Check database health
	if err := database.IsHealthy(db); err != nil {
		log.Fatal().Err(err).Msg("Database health check failed")
	}

	// Initialize service container
	serviceContainer, err := services.NewServiceContainer(db, cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize services")
	}

	// Create and configure server
	srv, err := server.New(cfg, serviceContainer)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create server")
	}

	// Setup signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		log.Info().
			Int("port", cfg.Server.Port).
			Str("host", cfg.Server.Host).
			Bool("tls", cfg.Server.TLSEnabled).
			Msg("Starting HTTP server")

		if err := srv.Start(); err != nil {
			serverErrors <- fmt.Errorf("server error: %w", err)
		}
	}()

	// Start background tasks
	go func() {
		log.Info().Msg("Starting background tasks")
		serviceContainer.StartBackgroundTasks(ctx)
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErrors:
		log.Error().Err(err).Msg("Server error occurred")
		stop()
	case <-ctx.Done():
		log.Info().Msg("Shutdown signal received")
	}

	// Graceful shutdown
	log.Info().Msg("Starting graceful shutdown...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	// Stop background tasks
	log.Info().Msg("Stopping background tasks")
	serviceContainer.StopBackgroundTasks()

	// Shutdown server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}

	// Final cleanup
	serviceContainer.Cleanup()

	log.Info().Msg("Microgateway stopped gracefully")
}

// setupLogging configures the global logger based on configuration
func setupLogging(cfg config.ObservabilityConfig) {
	// Set log level
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Configure output format
	if cfg.LogFormat == "text" {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		})
	} else {
		log.Logger = log.With().Timestamp().Logger()
	}

	// Set global timestamp format
	zerolog.TimeFieldFormat = time.RFC3339Nano
}