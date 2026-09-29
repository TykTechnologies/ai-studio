package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/docs"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/studio"
	"github.com/TykTechnologies/midsommar/v2/startup"
)

func printWelcome() {
	fmt.Printf("Starting Tyk AI Studio %v\n", Version)
	fmt.Printf("Copyright Tyk Technologies, %s\n", time.Now().Format("2006"))
}

func main() {
	printWelcome()

	// Parse command-line flags
	envFile := flag.String("env", "", "Path to environment file (default: .env in current directory)")
	noLLMDefaults := flag.Bool("no-llm-defaults", false, "Disable automatic creation of default LLM configurations and secrets")
	flag.Parse()

	// Get configuration first to initialize logger with correct level
	appConf := config.Get(*envFile)

	// Initialize logger with configured level
	logger.Init(appConf.LogLevel)
	logger.Infof("Log level set to: %s", appConf.LogLevel)

	// Report every configured path (grep 'startup path'); problems are WARNs.
	startup.ReportPaths(appConf, *envFile)

	// Perform connectivity tests before proceeding with initialization
	if err := startup.TestConnectivity(appConf); err != nil {
		logger.FatalErr("Connectivity tests failed", err)
	}

	db, err := openDatabase(appConf)
	if err != nil {
		logger.FatalErr("Failed to connect to the database", err)
	}
	logger.Info("Successfully connected to the database")

	gin.SetMode(gin.ReleaseMode)
	s, err := studio.New(studio.Options{
		Config:          appConf,
		DB:              db,
		Version:         Version,
		BuildHash:       BuildHash,
		BuildTime:       BuildTime,
		SkipLLMDefaults: *noLLMDefaults,
	})
	if err != nil {
		logger.FatalErr("Failed to start AI Studio", err)
	}

	// Setup signal handling for graceful shutdown
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Any server failing stops the process, as a signal would.
	serve := func(name string, run func() error) {
		go func() {
			if err := run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Errorf("%s error: %v", name, err)
				stop()
			}
		}()
	}

	// Start gateway if licensed (CE: always enabled, ENT: requires feature_gateway entitlement)
	go func() {
		err := s.StartProxy()
		switch {
		case errors.Is(err, studio.ErrGatewayNotLicensed):
			logger.Info("Gateway not started - feature_gateway not in license entitlements")
		case err != nil && !errors.Is(err, http.ErrServerClosed):
			logger.Errorf("Gateway error: %v", err)
		}
	}()

	if appConf.GatewayMode == "control" {
		logger.Infof("Starting AI Studio gRPC control server on port %d", appConf.GRPCPort)
		serve("gRPC control server", func() error { return s.StartGRPC(nil) })
	}

	if !docsDisabled(appConf) {
		go docs.NewServer(docsPort(appConf)).Start()
	}

	if !appConf.ProxyOnly {
		listenOn := fmt.Sprintf(":%s", appConf.ServerPort)
		logger.Infof("Server listening on %s", listenOn)
		serve("Server", func() error { return s.ListenAndServe(listenOn, appConf.CertFile, appConf.KeyFile) })
	} else {
		logger.Info("Running in proxy-only mode, waiting for shutdown signal...")
	}

	<-shutdownCtx.Done()
	logger.Info("Starting graceful shutdown...")

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Stop(cleanupCtx); err != nil {
		logger.Errorf("Error during shutdown: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			logger.Errorf("Failed to close database: %v", err)
		}
	}

	logger.Info("Application stopped gracefully")
}

// openDatabase connects to the configured database and checks it responds.
func openDatabase(appConf *config.AppConf) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch appConf.DatabaseType {
	case "sqlite":
		dialector = sqlite.Open(appConf.DatabaseURL)
	case "postgres":
		dialector = postgres.Open(appConf.DatabaseURL)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", appConf.DatabaseType)
	}

	db, err := gorm.Open(dialector, logger.GetGormConfig())
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// docsDisabled and docsPort apply the --no-docs and --docs-port arguments
// over the configuration.
func docsDisabled(appConf *config.AppConf) bool {
	for _, arg := range os.Args {
		if arg == "--no-docs" {
			return true
		}
	}
	return appConf.DocsDisabled
}

func docsPort(appConf *config.AppConf) int {
	for i, arg := range os.Args {
		if arg == "--docs-port" && i+1 < len(os.Args) {
			if port, err := strconv.Atoi(os.Args[i+1]); err == nil {
				return port
			}
		}
	}
	return appConf.DocsPort
}
