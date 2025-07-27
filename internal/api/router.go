package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"go-virtual-server/internal/config"
	"go-virtual-server/internal/database"
	"go-virtual-server/internal/services"
	"go-virtual-server/internal/util"

	// "go-virtual-server/internal/services"..

	_ "go-virtual-server/docs" // This line imports the generated docs.go

	httpSwagger "github.com/swaggo/http-swagger/v2" // Import the Swagger UI handler
)

// ServerAPI represents the API handlers and dependencies
type ServerAPI struct {
	cfg           *config.Config
	dbconn        *database.DBClient
	serverService *services.ServerService
	logger        *zap.Logger
	config        *config.Config
}

// NewServerAPI creates a new ServerAPI instance
func NewServerAPI(cfg *config.Config, dbClient *database.DBClient, serverService *services.ServerService, config *config.Config, logger *zap.Logger) *ServerAPI {
	return &ServerAPI{
		cfg:           cfg,
		dbconn:        dbClient,
		serverService: serverService,
		logger:        logger,
		config:        config,
	}
}

// Routes sets up the API routes
func (api *ServerAPI) Routes() http.Handler {
	route := chi.NewRouter()

	route.Use(middleware.RequestID)
	route.Use(middleware.RealIP)
	route.Use(util.StructuredLogger(util.GetLogger())) // Custom structured logger
	route.Use(middleware.Recoverer)

	// Basic CORS setup - adjust as needed for production
	route.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300, // Maximum value not ignored by any of all major browsers
	}))

	// --- Health Endpoints ---
	route.Get("/healthz", api.HealthzHandler)
	route.Get("/readyz", api.ReadyzHandler)

	// Prometheus metrics endpoint
	route.Handle("/metrics", promhttp.Handler())

	// --- API Endpoints ---
	// POST /servers
	route.Post("/server", api.ProvisionServer)
	// GET /servers
	route.Route("/servers", func(r chi.Router) {
		// GET /servers
		r.Get("/", api.ListServers)
		r.Route("/{serverID}", func(r chi.Router) {
			// POST /servers/:id/action
			r.Post("/action", api.PerformServerAction)
			// GET /servers/:id
			r.Get("/", api.GetServer)
			// GET /servers/:id/logs
			r.Get("/logs", api.GetServerLogs)
		})
	})
	// Swagger UI
	route.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	return route
}
