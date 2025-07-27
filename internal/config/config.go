package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// ServerPricingMap to store the price details for each type of servers.
type ServerPricingMap map[string]float64

// Config holds the application configuration.
type Config struct {
	HTTP_IP               string           `envconfig:"HTTP_IP" default:"0.0.0.0"`
	HTTPPort              int              `envconfig:"HTTP_PORT" default:"8080"`
	DBHost                string           `envconfig:"DB_HOST" default:"127.0.0.1"`
	DBPort                int              `envconfig:"DB_PORT" default:"5432"`
	DBUser                string           `envconfig:"DB_USER" default:"postgres"`
	DBPassword            string           `envconfig:"DB_PASSWORD" default:"mysecretpassword"`
	DBName                string           `envconfig:"DB_NAME" default:"postgres"`
	DBSSLMode             string           `envconfig:"DB_SSLMODE" default:"disable"`
	IPAllocationCIDR      string           `envconfig:"IP_ALLOCATION_CIDR" default:"192.168.0.0/24"`
	IPExclusionList       []string         `envconfig:"IP_EXCLUSION_LIST" default:""`
	LogLevel              string           `envconfig:"LOG_LEVEL" default:"info"`
	Environment           string           `envconfig:"ENVIRONMENT" default:"development"`
	LogFileCapacityInMB   int              `envconfig:"LOG_FILE_CAPACITY_IN_MB" default:"10"`
	DBMaxRetries          int              `envconfig:"DB_MAX_RETRIES" default:"10s"`
	DBRetryDelay          time.Duration    `envconfig:"DB_RETRY_DELAY" default:"5s"`
	BillingDaemonInterval time.Duration    `envconfig:"BILLING_DAEMON_INTERVAL" default:"1m"`
	ServerTypeWisePricing ServerPricingMap `envconfig:"SERVER_TYPE_WISE_PRICING" default:"micro:0.01,small:0.05,medium:0.10,large:0.20,xlarge:0.40"`
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	// Try to load environment variables from a .envconfig file in the user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	} else {
		dotEnvPath := filepath.Join(homeDir, ".env") 

		// godotenv.Load() will load variables from the .env file into the process's environment.
		err = godotenv.Load(dotEnvPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Now, process the environment variables (which now include those from .env, if loaded)
	// into our Config struct.
	var cfg Config
	err = envconfig.Process("", &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)

	}
	return &cfg, nil
}
