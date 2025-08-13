package repository

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/clickhouse"
)

type QueryRepository struct {
	client *clickhouse.Client
	logger *logrus.Logger
}

func NewQueryRepository(client *clickhouse.Client, logger *logrus.Logger) *QueryRepository {
	return &QueryRepository{
		client: client,
		logger: logger,
	}
}

// ExecuteQuery executes a query and returns results as a slice of maps
func (r *QueryRepository) ExecuteQuery(ctx context.Context, query string) ([]map[string]interface{}, int, time.Duration, error) {
	startTime := time.Now()

	// Use SQL DB connection for better compatibility
	db := r.client.GetDB()

	// Execute query
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, 0, time.Since(startTime), fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, 0, time.Since(startTime), fmt.Errorf("failed to get columns: %w", err)
	}

	// Get column types
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, 0, time.Since(startTime), fmt.Errorf("failed to get column types: %w", err)
	}

	// Prepare result slice
	var results []map[string]interface{}
	rowCount := 0

	// Create a slice of interface{} to hold pointers to each column value
	columnPointers := make([]interface{}, len(columns))
	columnValues := make([]interface{}, len(columns))

	for i := range columnValues {
		columnPointers[i] = &columnValues[i]
	}

	// Scan rows
	for rows.Next() {
		// Scan the row into our column pointers
		if err := rows.Scan(columnPointers...); err != nil {
			return nil, rowCount, time.Since(startTime), fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a map for this row
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			val := columnValues[i]

			// Handle special types and NULL values
			if val == nil {
				rowMap[col] = nil
			} else {
				// Convert based on the actual type
				rowMap[col] = r.convertValue(val, columnTypes[i])
			}
		}

		results = append(results, rowMap)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, rowCount, time.Since(startTime), fmt.Errorf("row iteration error: %w", err)
	}

	executionTime := time.Since(startTime)

	r.logger.WithFields(logrus.Fields{
		"query":          query,
		"row_count":      rowCount,
		"execution_time": executionTime,
	}).Info("Query executed successfully")

	return results, rowCount, executionTime, nil
}

// convertValue converts database values to appropriate Go types
func (r *QueryRepository) convertValue(value interface{}, columnType *sql.ColumnType) interface{} {
	if value == nil {
		return nil
	}

	// Get the database type name
	dbTypeName := columnType.DatabaseTypeName()

	// Handle ClickHouse specific types
	switch v := value.(type) {
	case []byte:
		// Convert byte arrays to strings
		return string(v)
	case time.Time:
		// Format times as RFC3339
		return v.Format(time.RFC3339)
	case *time.Time:
		if v != nil {
			return v.Format(time.RFC3339)
		}
		return nil
	case int64:
		// Check if this should be a uint based on the DB type
		if contains(dbTypeName, "UInt") {
			return uint64(v)
		}
		return v
	case float32, float64:
		return v
	case bool:
		return v
	case string:
		return v
	default:
		// For any other type, try to get the underlying value
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Ptr && !rv.IsNil() {
			return rv.Elem().Interface()
		}
		return value
	}
}

// ExecuteQueryNative executes a query using the native ClickHouse connection (alternative method)
func (r *QueryRepository) ExecuteQueryNative(ctx context.Context, query string) ([]map[string]interface{}, int, time.Duration, error) {
	startTime := time.Now()

	// Use native connection
	conn := r.client.GetConn()

	// Execute query
	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, 0, time.Since(startTime), fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns := rows.Columns()
	columnTypes := rows.ColumnTypes()

	// Prepare result slice
	var results []map[string]interface{}
	rowCount := 0

	// Scan rows
	for rows.Next() {
		// Create a slice to hold column values
		values := make([]interface{}, len(columns))

		// Create the appropriate type for each column based on its type
		for i, colType := range columnTypes {
			values[i] = r.createScanType(colType)
		}

		// Scan the row
		if err := rows.Scan(values...); err != nil {
			return nil, rowCount, time.Since(startTime), fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a map for this row
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			rowMap[col] = r.extractValue(values[i])
		}

		results = append(results, rowMap)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, rowCount, time.Since(startTime), fmt.Errorf("row iteration error: %w", err)
	}

	executionTime := time.Since(startTime)

	r.logger.WithFields(logrus.Fields{
		"query":          query,
		"row_count":      rowCount,
		"execution_time": executionTime,
	}).Info("Query executed successfully")

	return results, rowCount, executionTime, nil
}

// createScanType creates the appropriate scan type based on column type
func (r *QueryRepository) createScanType(colType interface{}) interface{} {
	// This is a simplified version - you might need to expand based on your ClickHouse types
	// For now, we'll use interface{} pointers for everything
	var v interface{}
	return &v
}

// extractValue extracts the actual value from a scanned pointer
func (r *QueryRepository) extractValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	// If it's a pointer to interface{}, dereference it
	if ptr, ok := value.(*interface{}); ok && ptr != nil {
		return r.convertSimpleValue(*ptr)
	}

	return r.convertSimpleValue(value)
}

// convertSimpleValue converts common types to JSON-friendly formats
func (r *QueryRepository) convertSimpleValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case []byte:
		return string(v)
	case time.Time:
		return v.Format(time.RFC3339)
	case *time.Time:
		if v != nil {
			return v.Format(time.RFC3339)
		}
		return nil
	default:
		return value
	}
}

// ValidateQuery performs basic query validation
func (r *QueryRepository) ValidateQuery(ctx context.Context, query string) error {
	// Use EXPLAIN to validate the query syntax
	explainQuery := fmt.Sprintf("EXPLAIN SYNTAX %s", query)

	db := r.client.GetDB()
	rows, err := db.QueryContext(ctx, explainQuery)
	if err != nil {
		return fmt.Errorf("query validation failed: %w", err)
	}
	defer rows.Close()

	// If we can get the explained query, it's valid
	var explained string
	if rows.Next() {
		if err := rows.Scan(&explained); err != nil {
			return fmt.Errorf("failed to scan EXPLAIN result: %w", err)
		}
	}

	return nil
}

// GetQueryPlan returns the execution plan for a query
func (r *QueryRepository) GetQueryPlan(ctx context.Context, query string) (string, error) {
	explainQuery := fmt.Sprintf("EXPLAIN %s", query)

	db := r.client.GetDB()
	rows, err := db.QueryContext(ctx, explainQuery)
	if err != nil {
		return "", fmt.Errorf("failed to get query plan: %w", err)
	}
	defer rows.Close()

	var plan string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", fmt.Errorf("failed to scan plan: %w", err)
		}
		plan += line + "\n"
	}

	return plan, rows.Err()
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && len(substr) > 0 &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				len(s) > len(substr)*2 && findSubstring(s, substr)))
}

// findSubstring checks if substr exists in s
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
