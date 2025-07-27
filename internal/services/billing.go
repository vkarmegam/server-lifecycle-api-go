package services

import (
	"context"
	"sync"
	"time"

	"github.com/go-chi/chi/middleware"
	"go.uber.org/zap"

	"go-virtual-server/internal/database/sqlc"
	"go-virtual-server/internal/util"
)

// BillingDaemon calculates and updates server uptime for billing purposes.
type BillingDaemon struct {
	queries  *sqlc.Queries
	logger   *zap.Logger
	interval time.Duration
	mutex    *sync.Mutex
}

// NewBillingAndReaperDaemon creates a new BillingDaemon.
func NewBillingAndReaperDaemon(queries *sqlc.Queries, logger *zap.Logger, interval time.Duration) *BillingDaemon {
	return &BillingDaemon{
		queries:  queries,
		logger:   logger,
		interval: interval,
	}
}

// Start kicks off the billing daemon's periodic processing.
func (billingDaemon *BillingDaemon) Start(ctx context.Context) {
	ticker := time.NewTicker(billingDaemon.interval)
	defer ticker.Stop()

	billingDaemon.logger.Info("Billing daemon started", zap.Duration("interval", billingDaemon.interval))
	for {
		select {
		case <-ctx.Done():
			billingDaemon.logger.Info("Billing daemon stopped due to context cancellation.")
			return
		case <-ticker.C:
			billingDaemon.processBilling(ctx)
		}
	}
}

// processBilling fetches running servers and updates their uptime.
func (billingDaemon *BillingDaemon) processBilling(ctx context.Context) {

	if billingDaemon.mutex == nil {
		billingDaemon.mutex = &sync.Mutex{}
	}
	billingDaemon.mutex.Lock()
	defer billingDaemon.mutex.Unlock()

	billingDaemon.logger.Debug("Running billing process...")

	// Fetch all running servers
	servers, err := billingDaemon.queries.ListServers(ctx, util.ServerStatusRunning)
	if err != nil {
		billingDaemon.logger.Error("Failed to list running servers for billing", zap.Error(err))
		return
	}
	billingDaemon.logger.Info("Found running servers", zap.Int("count", len(servers)))
	for _, server := range servers {
		billingDaemon.logger.Info("Processing server", zap.String("server_id", server.ID.String()))
		elapsed := time.Since(server.LastStatusUpdate.Time)
		newUptimeSeconds := server.UptimeSeconds + elapsed.Nanoseconds()/int64(time.Second)

		_, err := billingDaemon.queries.UpdateServerUptime(ctx, sqlc.UpdateServerUptimeParams{
			UptimeSeconds: newUptimeSeconds,
			ID:            server.ID,
		})

		if err != nil {
			billingDaemon.logger.Error("Failed to update server uptime",
				zap.Error(err),
				zap.String("server_id", server.ID.String()),
				zap.Int64("current_uptime", server.UptimeSeconds),
				zap.Int64("new_uptime", newUptimeSeconds),
			)
		} else {
			billingDaemon.logger.Info("Updated server uptime",
				zap.String("server_id", server.ID.String()),
				zap.Int64("new_uptime", newUptimeSeconds),
				zap.Float64("hourly_cost", server.HourlyCost),
				zap.Float64("estimated_current_bill", float64(newUptimeSeconds)/3600*float64(server.HourlyCost)),
			)
		}

		AppendServerLifecycleLogs(nil, billingDaemon, ctx, server.ID, []byte(`{"REQUEST_ID":"`+string(middleware.GetReqID(ctx))+`","ACTION": "Server uptime updated","SERVER_ID":"`+server.ID.String()+`","TIME":"`+time.Now().String()+`"}`))

		//  IDLE Reaper to terminate server if it is not used for more than 30 minmutes
		if newUptimeSeconds > 1800 && server.Status != util.ServerStatusTerminated {
			_, err := billingDaemon.queries.UpdateServerStatus(ctx, sqlc.UpdateServerStatusParams{
				Status: util.ServerStatusTerminated,
				ID:     server.ID,
			})
			if err != nil {
				billingDaemon.logger.Error("Failed to update server status to terminated",
					zap.Error(err),
					zap.String("server_id", server.ID.String()),
					zap.String("current_status", string(server.Status)),
					zap.String("desired_status", string(util.ServerStatusTerminated)),
				)
			} else {
				billingDaemon.logger.Info("Server status updated to terminated",
					zap.String("server_id", server.ID.String()),
					zap.String("current_status", string(server.Status)),
					zap.String("desired_status", string(util.ServerStatusTerminated)),
				)
				AppendServerLifecycleLogs(nil, billingDaemon, ctx, server.ID, []byte(`{"REQUEST_ID":"`+string(middleware.GetReqID(ctx))+`","ACTION": "Timeout detected, server status updated to terminated","SERVER_ID":"`+server.ID.String()+`","TIME":"`+time.Now().String()+`"}`))
			}
		}
	}
}
