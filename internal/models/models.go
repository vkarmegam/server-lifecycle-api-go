package models

import (
	"encoding/json"
	"time"

	"go-virtual-server/internal/database/sqlc"
)

// ProvisionServerRequest defines the request body for provisioning a server
type ProvisionServerRequest struct {
	Name   string `json:"name" example:"my-app-server"`
	Region string `json:"region" example:"us-east-1"`
	Type   string `json:"type" example:"t2.micro"`
}

// ServerActionRequest defines the request body for performing a server action
type ServerActionRequest struct {
	Action string `json:"action" example:"start"` // start, stop, reboot, terminate
}
type BillingInfo struct {
	BillingModel         string    `json:"billingModel" example:"hourly"`              // e.g., "hourly", "monthly", "per_request"
	CurrencyUnit         string    `json:"currencyUnit" example:"USD"`                 // e.g., "USD", "EUR", "GBP"
	UnitPrice            float64   `json:"unitPrice" example:"0.01"`                   // Price per unit (e.g., per hour, per request)
	UpdatedTime          time.Time `json:"updatedTime" example:"2023-10-27T09:00:00Z"` // Start of the current billing period
	TotalUptimeSeconds   int64     `json:"totalUptimeSeconds" example:"3600"`          // Total cumulative active time for the server
	EstimatedCurrentCost float64   `json:"estimatedCurrentCost" example:"0.01"`        // Estimated cost for the current (partial) billing cycle or total accumulated cost
}

// ServerResponse represents the response structure for a server
type ServerResponse struct {
	ID               string          `json:"id" example:"a1b2c3d4-e5f6-7890-1234-567890abcdef"`
	Name             string          `json:"name" example:"my-app-server"`
	Region           string          `json:"region" example:"us-east-1"`
	Status           string          `json:"status" example:"running"`
	Type             string          `json:"type" example:"t2.micro"`
	IPAddress        string          `json:"ipAddress" example:"192.168.1.10"`
	ProvisionedAt    time.Time       `json:"provisionedAt" example:"2023-10-27T10:00:00Z"`
	LastStatusUpdate time.Time       `json:"lastStatusUpdate" example:"2023-10-27T10:15:00Z"`
	UptimeSeconds    int64           `json:"uptimeSeconds" example:"900"`
	BillingInfo      BillingInfo     `json:"billingInfo"`
	HourlyCost       float64         `json:"hourlyCost" example:"0.01"`
	LifecycleLogs    json.RawMessage `json:"lifecycleLogs"`
	CreatedAt        time.Time       `json:"createdAt" example:"2023-10-27T09:55:00Z"`
	UpdatedAt        time.Time       `json:"updatedAt" example:"2023-10-27T10:15:00Z"`
}

// ListServersResponse for listing servers
type ListServersResponse struct {
	Servers []ServerResponse `json:"servers"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
}

// ServerLifecycleLogEntry represents a single entry in the server's lifecycle_logs JSONB array.
type ServerLifecycleLogEntry struct {
	RequestID string `json:"REQUEST_ID"`
	Action    string `json:"ACTION"`
	ServerID  string `json:"SERVER_ID"`
	Time      string `json:"TIME"`
} // ServerLogsResponse for server logs
type ServerLogsResponse struct {
	Logs []ServerLifecycleLogEntry `json:"logs"`
}

// ToServerResponse converts a sqlc.Server to a ServerResponse
func ToServerResponse(s sqlc.Server) ServerResponse {
	return ServerResponse{
		ID:               s.ID.String(),
		Name:             s.Name,
		Region:           s.Region,
		Status:           string(s.Status),
		Type:             string(s.Type),
		IPAddress:        s.Address,
		ProvisionedAt:    s.ProvisionedAt.Time,
		LastStatusUpdate: s.LastStatusUpdate.Time,
		UptimeSeconds:    s.UptimeSeconds,
		BillingInfo:      BillingInfo{},
		HourlyCost:       float64(s.HourlyCost),
		LifecycleLogs:    s.LifecycleLogs,
		CreatedAt:        s.CreatedAt.Time,
		UpdatedAt:        s.UpdatedAt.Time,
	}
}

// ToBillingInfo converts server uptime and hourly cost into a BillingInfo struct.
func ToBillingInfo(s sqlc.Server) BillingInfo {
	estimatedCost := (float64(s.UptimeSeconds) / 3600.0) * s.HourlyCost

	return BillingInfo{
		BillingModel:         "immediate",
		CurrencyUnit:         "USD",
		UnitPrice:            s.HourlyCost,
		UpdatedTime:          s.UpdatedAt.Time,
		TotalUptimeSeconds:   s.UptimeSeconds,
		EstimatedCurrentCost: estimatedCost,
	}
}
