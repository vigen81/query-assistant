package repository

import (
	"context"
	"fmt"
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

	// Use native connection instead of SQL DB
	conn := r.client.GetConn()

	// Execute query
	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, 0, time.Since(startTime), fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns := rows.Columns()

	// Get column types
	columnTypes := rows.ColumnTypes()

	// Prepare result slice
	var results []map[string]interface{}
	rowCount := 0

	// Scan rows
	for rows.Next() {
		// Create a slice to hold the actual values
		values := make([]interface{}, len(columns))
		// Create a slice of pointers to scan into
		valuePtrs := make([]interface{}, len(columns))

		// Initialize each value based on the column type
		for i, colType := range columnTypes {
			switch colType.DatabaseTypeName() {
			case "String", "FixedString":
				values[i] = new(string)
			case "UInt8", "UInt16", "UInt32", "UInt64":
				values[i] = new(uint64)
			case "Int8", "Int16", "Int32", "Int64":
				values[i] = new(int64)
			case "Float32":
				values[i] = new(float32)
			case "Float64":
				values[i] = new(float64)
			case "Date", "DateTime", "DateTime64":
				values[i] = new(time.Time)
			case "Bool":
				values[i] = new(bool)
			default:
				// For any other type, use interface{}
				values[i] = new(interface{})
			}
			valuePtrs[i] = values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, rowCount, time.Since(startTime), fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a map for this row
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			// Dereference the pointer and get the actual value
			var actualValue interface{}
			switch v := values[i].(type) {
			case *string:
				actualValue = *v
			case *uint64:
				actualValue = *v
			case *int64:
				actualValue = *v
			case *float32:
				actualValue = *v
			case *float64:
				actualValue = *v
			case *time.Time:
				actualValue = v.Format(time.RFC3339)
			case *bool:
				actualValue = *v
			case *interface{}:
				actualValue = *v
			default:
				actualValue = v
			}
			rowMap[col] = actualValue
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
func (r *QueryRepository) convertValue(value interface{}, columnType interface{}) interface{} {
	if value == nil {
		return nil
	}

	// Handle common ClickHouse types
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

	conn := r.client.GetConn()
	rows, err := conn.Query(ctx, explainQuery)
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

	conn := r.client.GetConn()
	rows, err := conn.Query(ctx, explainQuery)
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
