package util

import (
	"encoding/json"
	"net/http"
)

const (
	ServerStatusProvisioning = "provisioning"
	ServerStatusRunning      = "running"
	ServerStatusStopped      = "stopped"
	ServerStatusTerminated   = "terminated"
	ServerTypeT2Micro        = "t2.micro"
	ServerTypeM5Large        = "m5.large"
	ServerTypeC5Xlarge       = "c5.xlarge"
)

// ErrorResponse defines a generic error response structure.
type ErrorResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// IsValidTransition checks if a state transition is valid.
func IsValidTransition(currentStatus, desiredStatus string) bool {
	switch currentStatus {
	case ServerStatusProvisioning:
		return desiredStatus == ServerStatusRunning || desiredStatus == ServerStatusTerminated
	case ServerStatusRunning:
		return desiredStatus == ServerStatusStopped || desiredStatus == ServerStatusTerminated
	case ServerStatusStopped:
		return desiredStatus == ServerStatusRunning || desiredStatus == ServerStatusTerminated
	case ServerStatusTerminated:
		return false
	default:
		return false
	}
}

func IsValidServerType(serverType string) bool {
	switch serverType {
	case ServerTypeT2Micro, ServerTypeM5Large, ServerTypeC5Xlarge:
		return true
	default:
		return false
	}
}

// RespondWithJSON writes a JSON response with the given status code.
func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message": "Internal server error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

// RespondWithError writes an error JSON response.
func RespondWithError(w http.ResponseWriter, code int, message string) {
	RespondWithJSON(w, code, ErrorResponse{Message: message, Code: code})
}
