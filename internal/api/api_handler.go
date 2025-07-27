package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	// Added for time.Now()

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"go-virtual-server/internal/database/sqlc"
	"go-virtual-server/internal/models"
	"go-virtual-server/internal/services"
	"go-virtual-server/internal/util"
)

// ProvisionServer godoc
// @Summary Provision a new virtual server
// @Description Provisions a new virtual server with specified details.
// @Tags server
// @Accept json
// @Produce json
// @Param request body models.ProvisionServerRequest true "Server provision request"
// @Success 201 {object} models.ServerResponse
// @Failure 400 {object} util.ErrorResponse
// @Failure 409 {object} util.ErrorResponse
// @Failure 500 {object} util.ErrorResponse
// @Router /server [post]
func (api *ServerAPI) ProvisionServer(w http.ResponseWriter, r *http.Request) {

	api.logger.Info("Entering ProvisionServer handler")
	var req models.ProvisionServerRequest // Decode into models.ProvisionServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.logger.Error("Invalid request payload", zap.Error(err))
		util.RespondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Basic validation
	if req.Name == "" || req.Region == "" || req.Type == "" {
		api.logger.Warn("Missing required fields for server provisioning",
			zap.String("name", req.Name), zap.String("region", req.Region), zap.String("type", req.Type))
		util.RespondWithError(w, http.StatusBadRequest, "Name, region, and type are required")
		return
	}

	if !util.IsValidServerType(req.Type) {
		api.logger.Warn("Invalid server type provided", zap.String("type", req.Type))
		util.RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid server type. Only %s, %s, %s are allowed",
			util.ServerTypeC5Xlarge, util.ServerTypeM5Large, util.ServerTypeT2Micro))
		return
	}

	server, err := api.serverService.ProvisionNewServer(r.Context(), req.Name, req.Region, req.Type)
	if err != nil {
		api.logger.Error("Failed to provision server", zap.Error(err))
		util.RespondWithError(w, http.StatusInternalServerError, "Failed to provision server")
		return
	}

	// Respond with the full ServerResponse object
	response := models.ToServerResponse(server)
	api.logger.Info("New virtual server provisioned successfully",
		zap.String("server_id", response.ID),
		zap.String("server_name", response.Name))
	util.RespondWithJSON(w, http.StatusCreated, response)

	api.logger.Info("Exiting ProvisionServer handler")
}

// GetServer godoc
// @Summary Retrieve full metadata for a server
// @Description Retrieves full metadata for a specific virtual server, including live uptime and billing.
// @Tags servers
// @Produce json
// @Param serverID path string true "ID of the server"
// @Success 200 {object} models.ServerResponse
// @Failure 400 {object} util.ErrorResponse
// @Failure 404 {object} util.ErrorResponse
// @Failure 500 {object} util.ErrorResponse
// @Router /servers/{serverID} [get]
func (api *ServerAPI) GetServer(w http.ResponseWriter, r *http.Request) {

	api.logger.Info("Entering GetServer handler", zap.String("serverID", chi.URLParam(r, "serverID")))

	serverIDStr := chi.URLParam(r, "serverID")
	serverUUID := services.StringToPGUUID(serverIDStr) // Assuming this converts to pgtype.UUID

	// GetServer in sqlc.Queries should ideally return sqlc.Server directly
	// If it was a GetServerRow (with IP join), you'd use models.ToServerResponseFromGetServerRow
	server, err := api.dbconn.Queries.GetServer(r.Context(), serverUUID)
	if err != nil {
		api.logger.Error("Failed to retrieve server from database", zap.String("serverID", serverIDStr), zap.Error(err))
		util.RespondWithError(w, http.StatusInternalServerError, "Failed to retrieve server details")
		return
	}

	// Convert sqlc.Server to models.ServerResponse
	response := models.ToServerResponse(server)

	response.BillingInfo = models.ToBillingInfo(server)

	api.logger.Info("Successfully retrieved server details", zap.String("serverID", response.ID))
	util.RespondWithJSON(w, http.StatusOK, response)

	api.logger.Info("Exiting GetServer handler", zap.String("serverID", serverIDStr))
}

// PerformServerAction godoc
// @Summary Perform an action on a server
// @Description Performs actions like start, stop, reboot, terminate on a virtual server. Enforces valid FSM transitions.
// @Tags servers
// @Accept json
// @Produce json
// @Param serverID path string true "ID of the server"
// @Param request body models.ServerActionRequest true "Action to perform (start, stop, reboot, terminate)"
// @Success 200 {object} models.ServerResponse
// @Failure 400 {object} util.ErrorResponse
// @Failure 404 {object} util.ErrorResponse
// @Failure 409 {object} util.ErrorResponse
// @Failure 500 {object} util.ErrorResponse
// @Router /servers/{serverID}/action [post]
func (api *ServerAPI) PerformServerAction(w http.ResponseWriter, r *http.Request) {
	api.logger.Info("Entering PerformServerAction handler")

	var req models.ServerActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.logger.Error("Invalid request payload for server action", zap.Error(err))
		util.RespondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	serverIDStr := chi.URLParam(r, "serverID")
	serverUUID := services.StringToPGUUID(serverIDStr) // Convert to pgtype.UUID
	server, err := api.dbconn.Queries.GetServer(r.Context(), serverUUID)
	if err != nil {
		api.logger.Error("Failed to retrieve server for action", zap.String("serverID", serverIDStr), zap.Error(err))
		util.RespondWithError(w, http.StatusInternalServerError, "Failed to retrieve server details")
		return
	}

	var updatedServer sqlc.Server

	switch req.Action {
	case "start":
		updatedServer, err = api.serverService.StartServer(r.Context(), server)
	case "stop":
		updatedServer, err = api.serverService.StopServer(r.Context(), server)
	case "reboot":
		updatedServer, err = api.serverService.RebootServer(r.Context(), server)
	case "terminate":
		updatedServer, err = api.serverService.TerminateServer(r.Context(), server)
	default:
		api.logger.Warn("Invalid server action requested", zap.String("action", req.Action))
		util.RespondWithError(w, http.StatusBadRequest, "Invalid action: must be start, stop, reboot, or terminate")
		return
	}

	if err != nil {
		api.logger.Error("Failed to perform server action",
			zap.String("serverID", serverIDStr),
			zap.String("action", req.Action),
			zap.Error(err))
		util.RespondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to perform action %v: %v", req.Action, err))
		return
	}

	response := models.ToServerResponse(updatedServer)
	api.logger.Info("Server action completed successfully",
		zap.String("serverID", response.ID),
		zap.String("action", req.Action),
		zap.String("new_status", response.Status))
	util.RespondWithJSON(w, http.StatusOK, response)

	api.logger.Info("Exiting PerformServerAction handler")
}

// ListServers godoc
// @Summary List all servers
// @Description Lists all virtual servers, filterable by region, status, type; supports pagination (limit, offset); sorted (newest first).
// @Tags servers
// @Produce json
// @Param region query string false "Filter by region" example:"us-east-1"
// @Param status query string false "Filter by status (e.g., provisioning, running, stopped, terminated, error)" example:"running"
// @Param type query string false "Filter by server type (e.g., t2.micro, m5.large)" example:"t2.micro"
// @Param limit query int false "Number of results to return (default 10, max 100)" default(10) minimum(1) maximum(100)
// @Param offset query int false "Number of results to skip" default(0) minimum(0)
// @Success 200 {object} models.ListServersResponse
// @Failure 400 {object} util.ErrorResponse
// @Failure 500 {object} util.ErrorResponse
// @Router /servers [get]
func (api *ServerAPI) ListServers(w http.ResponseWriter, r *http.Request) {
	api.logger.Info("Entering ListServers handler")
	reqID := middleware.GetReqID(r.Context())
	api.logger.Info("Handling ListServers", zap.String("request_id", reqID))

	// Optional: set it in the response header
	w.Header().Set("X-Request-ID", reqID)

	regionParam := r.URL.Query().Get("region")
	statusParam := r.URL.Query().Get("status")
	typeParam := r.URL.Query().Get("type")

	limit := r.URL.Query().Get("limit")
	offset := r.URL.Query().Get("offset")

	// Start building the query
	baseQuery := `
        SELECT
            s.id, s.name, s.region, s.status, s.type, s.address,
            s.provisioned_at, s.last_status_update, s.uptime_seconds, s.hourly_cost, s.created_at, s.updated_at
        FROM servers s
    `
	conditions := []string{}
	args := []interface{}{}
	paramCounter := 0 // To track the placeholder number ($1, $2, etc.)

	if regionParam != "" {
		paramCounter++
		conditions = append(conditions, fmt.Sprintf("s.region = $%d", paramCounter))
		args = append(args, regionParam)
	}
	if statusParam != "" {
		paramCounter++
		conditions = append(conditions, fmt.Sprintf("s.status = $%d", paramCounter))
		args = append(args, statusParam)
	}
	if typeParam != "" {
		paramCounter++
		conditions = append(conditions, fmt.Sprintf("s.type = $%d", paramCounter))
		args = append(args, typeParam)
	}

	fullQuery := baseQuery
	if len(conditions) > 0 {
		fullQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	fullQuery += " ORDER BY s.created_at DESC"

	paramCounter++
	fullQuery += fmt.Sprintf(" LIMIT $%d", paramCounter)
	args = append(args, limit)

	paramCounter++
	fullQuery += fmt.Sprintf(" OFFSET $%d", paramCounter)
	args = append(args, offset)

	api.logger.Debug("Executing dynamic ListServers query",
		zap.String("query", fullQuery),
		zap.Any("args", args))

	rows, err := api.dbconn.Pool.Query(r.Context(), fullQuery, args...)
	if err != nil {
		api.logger.Error("Failed to execute query", zap.Error(err))
		util.RespondWithError(w, http.StatusInternalServerError, "Failed to retrieve servers")
		return
	}
	defer rows.Close()

	var servers []models.ServerResponse
	for rows.Next() {
		var s models.ServerResponse
		// Manually scan each column into the struct fields.
		// The order here MUST match the order in the SELECT statement.
		err := rows.Scan(
			&s.ID,
			&s.Name,
			&s.Region,
			&s.Status,
			&s.Type,
			&s.IPAddress,
			&s.ProvisionedAt,
			&s.LastStatusUpdate,
			&s.UptimeSeconds,
			&s.HourlyCost,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			api.logger.Error("Failed to scan server row", zap.Error(err))
			util.RespondWithError(w, http.StatusInternalServerError, "Failed to scan server data")
			return
		}

		servers = append(servers, s)
	}

	if err := rows.Err(); err != nil {
		api.logger.Error("Rows iteration error", zap.Error(err))
		util.RespondWithError(w, http.StatusInternalServerError, "Error processing server data")
		return
	}

	limitVal, err := strconv.Atoi(limit)
	if err != nil {
		limitVal = 10
	}
	offsetVal, err := strconv.Atoi(offset)
	if err != nil {
		offsetVal = 0
	}

	api.logger.Info("Successfully listed servers", zap.Int("count", len(servers)), zap.Any("query_params", r.URL.Query()))
	util.RespondWithJSON(w, http.StatusOK, models.ListServersResponse{
		Servers: servers,
		Total:   len(servers),
		Limit:   limitVal,
		Offset:  offsetVal,
	})

	api.logger.Info("Exiting ListServers handler")
}

// GetServerLogs godoc
// @Summary Return last 100 lifecycle events
// @Description Retrieves the last 100 lifecycle events for a specific virtual server.
// @Tags servers
// @Produce json
// @Param serverID path string true "ID of the server"
// @Success 200 {object} models.ServerLogsResponse
// @Failure 400 {object} util.ErrorResponse
// @Failure 404 {object} util.ErrorResponse
// @Failure 500 {object} util.ErrorResponse
// @Router /servers/{serverID}/logs [get]
func (api *ServerAPI) GetServerLogs(w http.ResponseWriter, r *http.Request) {

	api.logger.Info("Entering GetServerLogs handler")

	serverIDStr := chi.URLParam(r, "serverID")
	serverUUID := services.StringToPGUUID(serverIDStr) // Convert to pgtype.UUID
	server, err := api.dbconn.Queries.GetServer(r.Context(), serverUUID)
	if err != nil {
		api.logger.Error("Failed to retrieve server for action", zap.String("serverID", serverIDStr), zap.Error(err))
		util.RespondWithError(w, http.StatusInternalServerError, "Failed to retrieve server details")
		return
	}

	// server.LifecycleLogs is already json.RawMessage from GetServer query or ServerCtx
	var logs []models.ServerLifecycleLogEntry
	if len(server.LifecycleLogs) > 0 { // Only unmarshal if there's data
		if err := json.Unmarshal(server.LifecycleLogs, &logs); err != nil {
			api.logger.Error("Failed to parse server lifecycle logs from database", zap.String("serverID", server.ID.String()), zap.Error(err))
			util.RespondWithError(w, http.StatusInternalServerError, "Failed to parse server logs")
			return
		}
	} else {
		api.logger.Debug("No lifecycle logs found for server", zap.String("serverID", server.ID.String()))
		// logs is already an empty slice, which is fine
	}

	api.logger.Info("Successfully retrieved server lifecycle logs", zap.String("serverID", server.ID.String()), zap.Int("log_count", len(logs)))
	util.RespondWithJSON(w, http.StatusOK, models.ServerLogsResponse{Logs: logs})

	api.logger.Info("Exiting GetServerLogs handler")
}

// HealthzHandler godoc
// @Summary Application Liveness Probe
// @Description Checks if the application is alive and responding.
// @Tags Health
// @Produce plain
// @Success 200 {string} string "OK"
// @Router /healthz [get]
func (api *ServerAPI) HealthzHandler(w http.ResponseWriter, r *http.Request) {

	api.logger.Info("Entering HealthzHandler handler")
	util.RespondWithJSON(w, http.StatusOK, "OK")
	api.logger.Info("Exiting HealthzHandler handler")
}

// ReadyzHandler godoc
// @Summary Application Readiness Probe
// @Description Checks if the application is ready to serve traffic, including dependencies like the database.
// @Tags Health
// @Produce plain
// @Success 200 {string} string "OK"
// @Failure 503 {string} string "Service Unavailable"
// @Router /readyz [get]
func (api *ServerAPI) ReadyzHandler(w http.ResponseWriter, r *http.Request) {

	api.logger.Info("Entering ReadyzHandler handler")
	// Assuming dbconn.Pool (from database.DBClient) has a Ping method
	if api.dbconn == nil || api.dbconn.Pool == nil { // Check if DBClient or Pool is nil
		api.logger.Error("Readyz check failed: Database client or pool is nil.")
		util.RespondWithJSON(w, http.StatusServiceUnavailable, "Database client or pool is nil.")
		return
	}

	if err := api.dbconn.Pool.Ping(r.Context()); err != nil {
		api.logger.Error("Readyz check failed: Database not reachable", zap.Error(err))
		util.RespondWithJSON(w, http.StatusServiceUnavailable, "Database not reachable")
		return
	}

	// Add other critical dependency checks here if needed (e.g., message queues, external APIs)
	util.RespondWithJSON(w, http.StatusOK, "OK")
	api.logger.Info("Exiting ReadyzHandler handler")
}
