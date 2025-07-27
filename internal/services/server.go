package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"go-virtual-server/internal/config"
	"go-virtual-server/internal/database/sqlc"
	"go-virtual-server/internal/util"
)

// ServerService handles business logic related to servers.
type ServerService struct {
	queries     *sqlc.Queries
	ipAllocator *IPAllocator
	logger      *zap.Logger
	config      *config.Config
}

// NewServerService creates a new ServerService.
func NewServerService(queries *sqlc.Queries, ipAllocator *IPAllocator, logger *zap.Logger, config *config.Config) *ServerService {
	return &ServerService{
		queries:     queries,
		ipAllocator: ipAllocator,
		logger:      logger,
		config:      config,
	}
}

// ProvisionNewServer handles the logic for provisioning a new server.
func (s *ServerService) ProvisionNewServer(ctx context.Context, name string, region string, serverType string) (sqlc.Server, error) {

	s.logger.Info("Attempting to provision new server",
		zap.String("name", name),
		zap.String("region", region),
		zap.String("type", string(serverType)),
	)

	// 1. Allocate IP Address
	allocatedIP, err := s.ipAllocator.AllocateIP(ctx)
	if err != nil {
		s.logger.Error("Failed to allocate IP address", zap.Error(err))
		return sqlc.Server{}, errors.New("failed to allocate IP address")
	}
	hourlyConst := 0.1

	if _, ok := s.config.ServerTypeWisePricing[serverType]; ok {
		hourlyConst = s.config.ServerTypeWisePricing[serverType]
	}

	// 2. Create Server in DB
	createServerParams := sqlc.CreateNewServerParams{
		Name:       name + "_" + allocatedIP.Address,
		Region:     region,
		Type:       serverType,
		HourlyCost: hourlyConst,
		Address:    allocatedIP.Address, // pgtype.UUIDallocatedIP.Address,
		Status:     util.ServerStatusProvisioning,
	}
	server, err := s.queries.CreateNewServer(ctx, createServerParams)
	if err != nil {
		s.logger.Error("Failed to create server in DB", zap.Error(err), zap.String("ip_id", allocatedIP.ID.String()))
		// Important: If server creation fails, deallocate the IP!
		return sqlc.Server{}, fmt.Errorf("failed to create server: %+v", err)
	}

	deallocateErr := s.ipAllocator.saveAllocatedIP(ctx, server.ID, allocatedIP.ID)
	if deallocateErr != nil {
		s.logger.Error("Failed to deallocate IP after server creation failure", zap.Error(deallocateErr), zap.String("ip_id", allocatedIP.ID.String()))
	}

	_, err = s.queries.AppendServerLifecycleLog(ctx, sqlc.AppendServerLifecycleLogParams{
		Column1: []byte(`{"REQUEST_ID":"` + string(middleware.GetReqID(ctx)) + `","ACTION": "Server provisioned successfully","SERVER_ID":"` + server.ID.String() + `","TIME":"` + time.Now().String() + `"}`),
		ID:      server.ID,
	})
	if err != nil {
		s.logger.Warn("Failed to append initial provisioning lifecycle log",
			zap.Error(err),
			zap.String("server_id", server.ID.String()),
			zap.String("request_id", middleware.GetReqID(ctx)),
		)
	}

	s.logger.Info("Server provisioned successfully",
		zap.String("server_id", server.ID.String()),
		zap.String("ip_address", allocatedIP.Address),
	)

	return server, nil
}

// StartServer changes server status to running.
func (s *ServerService) StartServer(ctx context.Context, server sqlc.Server) (sqlc.Server, error) {

	if !util.IsValidTransition(server.Status, util.ServerStatusRunning) {
		s.logger.Warn("Invalid state transition attempt",
			zap.String("server_id", server.ID.String()),
			zap.String("current_status", string(server.Status)),
			zap.String("desired_status", string(util.ServerStatusRunning)),
		)
		return sqlc.Server{}, fmt.Errorf("%+v from %s to %s", "invalid state transition", server.Status, util.ServerStatusRunning)
	}

	updatedServer, err := s.queries.UpdateServerStatus(ctx, sqlc.UpdateServerStatusParams{
		Status: util.ServerStatusRunning,
		ID:     server.ID,
	})
	if err != nil {
		s.logger.Error("Failed to update server status to running", zap.Error(err), zap.String("server_id", server.ID.String()))
		return sqlc.Server{}, err
	}

	//  to update application logs and maintain the ;limit of the logs
	//
	err = AppendServerLifecycleLogs(s, nil, ctx, server.ID, []byte(`{"REQUEST_ID":"`+string(middleware.GetReqID(ctx))+`","ACTION": "Server start initiated","SERVER_ID":"`+server.ID.String()+`","TIME":"`+time.Now().String()+`"}`))

	if err != nil {
		s.logger.Warn("Failed to append reboot log", zap.Error(err), zap.String("server_id", server.ID.String()))
	}

	s.logger.Info("Server started", zap.String("server_id", server.ID.String()))
	return updatedServer, nil
}

// StopServer changes server status to stopped.
func (s *ServerService) StopServer(ctx context.Context, server sqlc.Server) (sqlc.Server, error) {
	if !util.IsValidTransition(server.Status, util.ServerStatusStopped) {
		s.logger.Warn("Invalid state transition attempt",
			zap.String("server_id", server.ID.String()),
			zap.String("current_status", string(server.Status)),
			zap.String("desired_status", string(util.ServerStatusStopped)),
		)
		return sqlc.Server{}, fmt.Errorf("%+v from %s to %s", "invalid state transition", server.Status, util.ServerStatusStopped)
	}

	updatedServer, err := s.queries.UpdateServerStatus(ctx, sqlc.UpdateServerStatusParams{
		Status: util.ServerStatusStopped,
		ID:     server.ID,
	})

	if err != nil {
		s.logger.Error("Failed to update server status to stopped", zap.Error(err), zap.String("server_id", server.ID.String()))
		return sqlc.Server{}, err
	}

	//  to update application logs and maintain the ;limit of the logs
	//
	err = AppendServerLifecycleLogs(s, nil, ctx, server.ID, []byte(`{"REQUEST_ID":"`+string(middleware.GetReqID(ctx))+`","ACTION": "Server stop initiated","SERVER_ID":"`+server.ID.String()+`","TIME":"`+time.Now().String()+`"}`))

	if err != nil {
		s.logger.Warn("Failed to append reboot log", zap.Error(err), zap.String("server_id", server.ID.String()))
	}

	s.logger.Info("Server stopped", zap.String("server_id", server.ID.String()))
	return updatedServer, nil
}

// RebootServer simulates a reboot (stop then start).
func (s *ServerService) RebootServer(ctx context.Context, server sqlc.Server) (sqlc.Server, error) {
	// A reboot is often an intermediate state in real systems.
	// For simplicity, we'll model it as a direct transition to running,
	// potentially with a brief 'rebooting' log.
	if server.Status == util.ServerStatusTerminated {
		s.logger.Warn("Cannot reboot server in terminal state", zap.String("server_id", server.ID.String()), zap.String("status", string(server.Status)))
		return sqlc.Server{}, fmt.Errorf("%+v: cannot reboot from %s", "invalid state transition", server.Status)
	}

	// Log the reboot initiation
	err := AppendServerLifecycleLogs(s, nil, ctx, server.ID, []byte(`{"REQUEST_ID":"`+string(middleware.GetReqID(ctx))+`","ACTION": "Server reboot initiated","SERVER_ID":"`+server.ID.String()+`","TIME":"`+time.Now().String()+`"}`))

	if err != nil {
		s.logger.Warn("Failed to append reboot log", zap.Error(err), zap.String("server_id", server.ID.String()))
	}

	updatedServer, err := s.queries.UpdateServerStatus(ctx, sqlc.UpdateServerStatusParams{
		Status: util.ServerStatusRunning, // Assuming reboot completes to running state
		ID:     server.ID,
	})
	if err != nil {
		s.logger.Error("Failed to update server status to running after reboot", zap.Error(err), zap.String("server_id", server.ID.String()))
		return sqlc.Server{}, err
	}
	s.logger.Info("Server rebooted", zap.String("server_id", server.ID.String()))
	return updatedServer, nil
}

// TerminateServer changes server status to terminated and deallocates IP.
func (s *ServerService) TerminateServer(ctx context.Context, server sqlc.Server) (sqlc.Server, error) {
	if !util.IsValidTransition(server.Status, util.ServerStatusTerminated) {
		s.logger.Warn("Invalid state transition attempt",
			zap.String("server_id", server.ID.String()),
			zap.String("current_status", string(server.Status)),
			zap.String("desired_status", string(util.ServerStatusTerminated)),
		)
		return sqlc.Server{}, fmt.Errorf("%+v from %s to %s", "invalid state transition", server.Status, util.ServerStatusTerminated)
	}

	// Update server status to terminated
	_, err := s.queries.UpdateServerStatus(ctx, sqlc.UpdateServerStatusParams{
		Status: util.ServerStatusTerminated,
		ID:     server.ID,
	})
	if err != nil {
		return sqlc.Server{}, fmt.Errorf("failed to update server status to terminated: %+v", err)
	}

	// Deallocate IP address
	ipData, err := s.queries.GetIPAddressByServerID(ctx, server.ID)
	if err != nil {
		return sqlc.Server{}, fmt.Errorf("failed to get IP address by server ID: %+v", err)
	}
	_, err = s.queries.DeallocateIPAddress(ctx, ipData.ID)
	if err != nil {
		return sqlc.Server{}, fmt.Errorf("failed to deallocate IP address: %+v", err)
	}

	if err != nil {
		s.logger.Error("Failed to terminate server or deallocate IP", zap.Error(err), zap.String("server_id", server.ID.String()))
		return sqlc.Server{}, err
	}

	s.logger.Info("Server terminated and IP deallocated", zap.String("server_id", server.ID.String()))

	//  to update application logs and maintain the ;limit of the logs
	//
	err = AppendServerLifecycleLogs(s, nil, ctx, server.ID, []byte(`{"REQUEST_ID":"`+string(middleware.GetReqID(ctx))+`","ACTION": "Server terminated","SERVER_ID":"`+server.ID.String()+`","TIME":"`+time.Now().String()+`"}`))

	if err != nil {
		s.logger.Warn("Failed to append reboot log", zap.Error(err), zap.String("server_id", server.ID.String()))
	}

	// Re-fetch the server to return the updated state (if you need the full updated object)
	// Or simply return the original server with updated status if that's sufficient
	updatedServer, err := s.queries.GetServer(ctx, server.ID)
	if err != nil {
		s.logger.Error("Failed to re-fetch terminated server", zap.Error(err), zap.String("server_id", server.ID.String()))
		return sqlc.Server{}, err // Return original server and error
	}
	return updatedServer, nil

}

// AppendServerLifecycleLogs updates the lifecycle_logs JSONB array for a server.
// It appends a new log entry, rotating if the array exceeds 100 entries.
// It returns the updated lifecycle_logs as []byte (JSONB) and an error.
func AppendServerLifecycleLogs(s *ServerService, bd *BillingDaemon, ctx context.Context, serverID pgtype.UUID, message []byte) error {

	if s.queries != nil {
		updatedLogs, err := s.queries.AppendServerLifecycleLog(ctx, sqlc.AppendServerLifecycleLogParams{
			Column1: message,
			ID:      serverID,
		})
		if err != nil {
			s.logger.Error("Failed to append lifecycle log", zap.Error(err), zap.String("server_id, ", serverID.String()))
			return err
		}

		if len(updatedLogs) > 100 {
			err := s.queries.EnforceLifecycleLogsLimit(ctx, serverID)
			if err != nil {
				s.logger.Error("Failed to append lifecycle log", zap.Error(err), zap.String("server_id", serverID.String()))
				return err
			}
		}
		return nil
	} else {
		updatedLogs, err := bd.queries.AppendServerLifecycleLog(ctx, sqlc.AppendServerLifecycleLogParams{
			Column1: message,
			ID:      serverID,
		})
		if err != nil {
			bd.logger.Error("Failed to append lifecycle log", zap.Error(err), zap.String("server_id, ", serverID.String()))
			return err
		}

		if len(updatedLogs) > 100 {
			err := bd.queries.EnforceLifecycleLogsLimit(ctx, serverID)
			if err != nil {
				bd.logger.Error("Failed to append lifecycle log", zap.Error(err), zap.String("server_id", serverID.String()))
				return err
			}
		}
	}
	return nil

}

// StringToPGUUID : function to convert string to pgtype.UUID
func StringToPGUUID(s string) pgtype.UUID {
	var pgUUID pgtype.UUID

	// Parse the string to a UUID
	u, err := uuid.Parse(s)
	if err != nil {
		return pgUUID
	}

	// Convert to pgtype.UUID
	pgUUID = pgtype.UUID{
		Bytes: u,
		Valid: true,
	}

	return pgUUID
}
