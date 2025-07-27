package database

import (
	"context"
	"fmt"
	"go-virtual-server/internal/database/sqlc"
	"time"

	"github.com/jackc/pgx/v5/pgxpool" 
	"go.uber.org/zap"
)

// DBClient holds the database connection pool and sqlc queries.
type DBClient struct {
	Pool    *pgxpool.Pool
	Queries *sqlc.Queries
}

// NewDBClient initializes a new database client with connection retry logic.
func NewDBClient(ctx context.Context, databaseURL string, maxRetries int, retryDelay time.Duration, logger *zap.Logger) (*DBClient, error) {
	var connPool *pgxpool.Pool
	var err error

	for i := 0; i < maxRetries; i++ {
		logger.Info("Attempting to connect to database", zap.Int("attempt", i+1), zap.Int("max_attempts", maxRetries))

		connPool, err = pgxpool.New(ctx, databaseURL)
		if err == nil {
			// checking the health of the connection pool.
			err = connPool.Ping(ctx)
			if err == nil {
				logger.Info("Successfully connected to database")
				return &DBClient{
					Pool:    connPool,
					Queries: sqlc.New(connPool),
				}, nil
			}
		}

		logger.Warn("Failed to connect to database, retrying...", zap.Error(err), zap.Duration("delay", retryDelay))
		time.Sleep(retryDelay) 
		// Wait before the next retry
	}

	return nil, fmt.Errorf("failed to connect to database after %d retries: %w", maxRetries, err)
}

// Close closes the database connection pool.
func (db *DBClient) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}
