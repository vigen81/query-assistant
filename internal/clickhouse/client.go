package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/config"
)

type Client struct {
	conn   driver.Conn
	db     *sql.DB
	config *config.Config
	logger *logrus.Logger
}

func NewClient(cfg *config.Config, logger *logrus.Logger) (*Client, error) {
	// Set default values if not specified
	maxOpenConns := cfg.ClickHouse.MaxOpenConns
	if maxOpenConns <= 0 {
		maxOpenConns = 10
	}
	maxIdleConns := cfg.ClickHouse.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = 5
	}
	connMaxLifetime := cfg.ClickHouse.ConnMaxLifetime
	if connMaxLifetime <= 0 {
		connMaxLifetime = 300
	}

	// ClickHouse connection options
	options := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", cfg.ClickHouse.Host, cfg.ClickHouse.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.ClickHouse.Database,
			Username: cfg.ClickHouse.Username,
			Password: cfg.ClickHouse.Password,
		},
		DialTimeout: 5 * time.Second,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
	}

	// Create native connection
	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("failed to open clickhouse connection: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	// Create standard SQL DB for queries
	// IMPORTANT: We need to create a new options struct for the SQL connection
	sqlOptions := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", cfg.ClickHouse.Host, cfg.ClickHouse.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.ClickHouse.Database,
			Username: cfg.ClickHouse.Username,
			Password: cfg.ClickHouse.Password,
		},
		DialTimeout: 5 * time.Second,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
	}

	// Open SQL database connection
	db := clickhouse.OpenDB(sqlOptions)

	// IMPORTANT: Set connection pool parameters BEFORE using the connection
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(time.Duration(connMaxLifetime) * time.Second)

	// Test the SQL connection with a simple ping
	// This is important to ensure the connection pool is properly initialized
	if err := db.Ping(); err != nil {
		// If ping fails, it might be a transient issue, log but don't fail
		logger.WithError(err).Warn("Initial SQL DB ping failed, will retry on first use")
	}

	logger.WithFields(logrus.Fields{
		"host":              cfg.ClickHouse.Host,
		"port":              cfg.ClickHouse.Port,
		"database":          cfg.ClickHouse.Database,
		"max_open_conns":    maxOpenConns,
		"max_idle_conns":    maxIdleConns,
		"conn_max_lifetime": connMaxLifetime,
	}).Info("ClickHouse connection established")

	return &Client{
		conn:   conn,
		db:     db,
		config: cfg,
		logger: logger,
	}, nil
}

// Query executes a query and returns results
func (c *Client) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	// Add timeout to context
	timeout := c.config.GetQueryTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c.logger.WithFields(logrus.Fields{
		"query":   query,
		"timeout": timeout,
	}).Debug("Executing ClickHouse query")

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	return rows, nil
}

// QueryRow executes a query and returns a single row
func (c *Client) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	timeout := c.config.GetQueryTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.db.QueryRowContext(ctx, query, args...)
}

// GetConn returns the native ClickHouse connection
func (c *Client) GetConn() driver.Conn {
	return c.conn
}

// GetDB returns the SQL database connection
func (c *Client) GetDB() *sql.DB {
	return c.db
}

// Close closes the database connections
func (c *Client) Close() error {
	if err := c.conn.Close(); err != nil {
		c.logger.WithError(err).Error("Failed to close ClickHouse connection")
	}
	if err := c.db.Close(); err != nil {
		c.logger.WithError(err).Error("Failed to close ClickHouse SQL connection")
	}
	return nil
}

// HealthCheck performs a health check on the ClickHouse connection
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var result int
	err := c.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected health check result: %d", result)
	}

	return nil
}
