package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"go-virtual-server/internal/api"
	"go-virtual-server/internal/config"
	"go-virtual-server/internal/database" // Ensure this is imported
	"go-virtual-server/internal/services"
	"go-virtual-server/internal/util"
)

var logger *zap.Logger

// @title Virtual Server Management API
// @version 1.0
// @description This is a virtual server management API built with Go, Chi, and PostgreSQL.
// @description
// @description
// @description  to View Methods go to http://localhost:8080/metrics
// @description
// @description karmegam vadivel
// @description https://github.com/vkarmegam
// @description https://www.linkedin.com/in/karmegamv
// @host localhost:8080
// @BasePath /

// go:generate swag init --parseDependency --parseInternal
func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	if err := util.InitLogger(cfg.LogLevel, cfg.Environment, cfg.LogFileCapacityInMB); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	logger = util.GetLogger()
	defer func() {
		if err := logger.Sync(); err != nil && err.Error() != "sync /dev/stderr: invalid argument" {
			fmt.Fprintf(os.Stderr, "Error syncing logger: %v\n", err)
		}
	}()

	logger.Info("--- Virtual Server Application (Simulation) ---")
	logger.Info("Log Level configured", zap.String("level", cfg.LogLevel))
	logger.Debug("Application configuration",
		zap.String("environment", cfg.Environment),
		zap.String("http_ip", cfg.HTTP_IP),
		zap.Int("http_port", cfg.HTTPPort),
		zap.String("db_host", cfg.DBHost),
		zap.Int("db_port", cfg.DBPort),
		zap.String("db_user", cfg.DBUser),
		zap.String("db_name", cfg.DBName),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	databaseURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
		cfg.DBSSLMode,
	)

	dbClient, err := database.NewDBClient(ctx, databaseURL, cfg.DBMaxRetries, cfg.DBRetryDelay, logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer dbClient.Close()
	logger.Info("Database connection established.")

	// queries  // Initialize sqlc queries object
	dbCleanup := services.NewIPAllocator(dbClient.Queries, logger)

	// Pre-populate IP pool in the database (this will always run after clearing data)
	if err := dbCleanup.TerminateAllServers(ctx, cfg.IPAllocationCIDR, cfg.IPExclusionList); err != nil {
		logger.Fatal("Failed to pre-populate IP pool", zap.Error(err), zap.String("cidr", cfg.IPAllocationCIDR))
	} else {
		logger.Info("IP pool pre-populated successfully", zap.String("cidr", cfg.IPAllocationCIDR))
	}

	// Start a Go routine to run the billing and reaper daemon
	billingAndReaperDaemon := services.NewBillingAndReaperDaemon(dbClient.Queries, logger, cfg.BillingDaemonInterval)
	go billingAndReaperDaemon.Start(ctx)
	logger.Info("Billing and Reaper daemon started in background", zap.Duration("interval", cfg.BillingDaemonInterval))

	// Start a Go routine to update Prometheus metrics
	metricsUpdater := services.NewMetricsUpdater(ctx, cancel, dbClient.Queries, cfg, logger)
	go metricsUpdater.Start(ctx)
	logger.Info("Metrics updater started in background")

	// Initialize server API
	serverAPI := api.NewServerAPI(cfg, dbClient, services.NewServerService(dbClient.Queries, dbCleanup, logger, cfg), cfg, logger)
	router := serverAPI.Routes()

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.HTTP_IP, cfg.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Info("HTTP server starting", zap.String("address", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server failed to start", zap.Error(err))
		}
	}()

	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM)
	<-stopSignal

	logger.Info("Shutting down HTTP server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown failed", zap.Error(err))
	} else {
		logger.Info("HTTP server gracefully stopped")
	}

	cancel()
	logger.Info("Application exiting")
}
