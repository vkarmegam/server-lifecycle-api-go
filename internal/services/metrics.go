package services

import (
	"context"
	"log"
	"time"

	"go-virtual-server/internal/config" // Make sure to adjust 'your-module-name' to your actual Go module name
	"go-virtual-server/internal/database/sqlc"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// These are the Prometheus metrics we will track for our server application.
var (
	// serverTotal is a Gauge that shows the total number of servers ever created.
	serverTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "server_total",
			Help: "Total number of servers ever created.",
		},
	)

	// serverCurrentStatusCount is a GaugeVec that counts servers by their current status (e.g., "running", "stopped").
	serverCurrentStatusCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "server_current_status",
			Help: "Current  servers by status (e.g.,provisioning : 0 running : 1, stopped : 2, terminated : 3).",
		},
		[]string{"status"},
	)

	// serverHourlyCost is a CounterVec that tracks the total number of lifecycle events.
	serverHourlyCost = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "server_hourly_cost",
			Help: "Current server usage cost",
		},
		[]string{"event_type"},
	)

	// serverUptimeSeconds is a GaugeVec that shows the current uptime in seconds for each running server.
	// It's labeled by the specific server's ID.
	serverUptimeSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "server_uptime_seconds",
			Help: "Current uptime of running servers in seconds.",
		},
		[]string{"server_id"},
	)
)

// The init() function runs automatically when the package is loaded.
// It's used here to register all our defined Prometheus metrics with the default registry.
// This makes them available for Prometheus to scrape from the /metrics endpoint.
func init() {
	prometheus.MustRegister(serverTotal)
	prometheus.MustRegister(serverCurrentStatusCount)
	prometheus.MustRegister(serverHourlyCost)
	prometheus.MustRegister(serverUptimeSeconds)
}

// MetricsUpdater is a struct that manages updating our Prometheus metrics.
// It holds references to the server repository (to get server data),
// the application configuration (for check intervals), and a context for graceful shutdown.
type MetricsUpdater struct {
	queries *sqlc.Queries
	logger  *zap.Logger
	config  *config.Config     // Application configuration (e.g., how often to check)
	ctx     context.Context    // Context for graceful shutdown
	cancel  context.CancelFunc // Function to signal shutdown for this updater
}

// NewMetricsUpdater creates and returns a new MetricsUpdater instance.
// It sets up a context that can be cancelled to stop the updater.
func NewMetricsUpdater(ctx context.Context, cancel context.CancelFunc, queries *sqlc.Queries, cfg *config.Config, logger *zap.Logger) *MetricsUpdater {

	return &MetricsUpdater{
		queries: queries,
		logger:  logger,
		config:  cfg,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start begins the metrics updater's background process.
// It runs in its own goroutine, periodically calling updateMetrics.
func (mu *MetricsUpdater) Start(ctx context.Context) {
	mu.logger.Info("Metrics Updater started. Updating metrics every 30 seconds")
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop() // Ensure the ticker is stopped when the goroutine exits

		for {
			select {
			case <-ticker.C: // When the ticker "ticks" (sends a signal)
				mu.updateMetrics() // Call the function to update all metrics
			case <-mu.ctx.Done(): // If the context is cancelled (e.g., app shutting down)
				log.Println("Metrics Updater shutting down.")
				return
			}
		}
	}()
}

// Stop signals the metrics updater to shut down gracefully.
// It does this by cancelling its context.
func (mu *MetricsUpdater) Stop() {
	mu.cancel() // Call the cancel function associated with the updater's context
}

// updateMetrics retrieves current server data and updates all Prometheus gauges.
func (mu *MetricsUpdater) updateMetrics() {

	log.Println("Metrics Updater: Updating server metrics...")

	// Get all servers from the repository.
	servers, err := mu.queries.SelectAllServers(mu.ctx)
	if err != nil {
		log.Printf("Metrics Updater: Error getting all servers: %v", err)
		return // Stop this update cycle if we can't get server data
	}

	// Reset
	serverCurrentStatusCount.Reset()
	serverUptimeSeconds.Reset()
	serverHourlyCost.Reset()

	// Set the total server count gauge.
	serverTotal.Set(float64(len(servers)))
	// Loop through each server and update the relevant metrics.
	for _, srv := range servers {
		serverCurrentStatusCount.WithLabelValues(srv.Name).Set(float64(getStatusNumber(srv.Status)))
		serverHourlyCost.WithLabelValues(srv.Name).Add(srv.HourlyCost)
		serverUptimeSeconds.WithLabelValues(srv.Name).Set(float64(srv.UptimeSeconds))
	}
}

// getStatusNumber returns a number corresponding to the server's current status.
func getStatusNumber(status string) float64 {
	switch status {
	case "running":
		return 1
	case "stopped":
		return 2
	case "terminated":
		return 3
	default:
		return 0
	}
}
