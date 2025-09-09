package repository

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/clickhouse"
	"gitlab.smartbet.am/golang/query-assistant/internal/models"
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

// ExecuteQueryWithMetadata executes a query and returns results with column metadata
func (r *QueryRepository) ExecuteQueryWithMetadata(ctx context.Context, query string, page, pageSize int) (
	results []map[string]interface{},
	columns []models.ColumnMetadata,
	totalRows int64,
	executionTime time.Duration,
	err error,
) {
	startTime := time.Now()

	// Skip total count for now - it can be expensive and cause timeouts
	// In production, you'd want to cache this or use approximate counts
	totalRows = -1 // Indicate unknown total

	// Modify query for pagination
	paginatedQuery := r.addPagination(query, page, pageSize)

	// Use SQL DB connection
	db := r.client.GetDB()

	// Execute query
	rows, err := db.QueryContext(ctx, paginatedQuery)
	if err != nil {
		return nil, nil, totalRows, time.Since(startTime), fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column metadata
	columns, err = r.extractColumnMetadata(rows)
	if err != nil {
		return nil, nil, totalRows, time.Since(startTime), fmt.Errorf("failed to get column metadata: %w", err)
	}

	// Get column names for scanning
	columnNames, err := rows.Columns()
	if err != nil {
		return nil, nil, totalRows, time.Since(startTime), fmt.Errorf("failed to get columns: %w", err)
	}

	// Get column types
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, nil, totalRows, time.Since(startTime), fmt.Errorf("failed to get column types: %w", err)
	}

	// Prepare result slice
	results = []map[string]interface{}{}
	rowCount := 0

	// Create a slice of interface{} to hold pointers to each column value
	columnPointers := make([]interface{}, len(columnNames))
	columnValues := make([]interface{}, len(columnNames))

	for i := range columnValues {
		columnPointers[i] = &columnValues[i]
	}

	// Scan rows
	for rows.Next() {
		if err := rows.Scan(columnPointers...); err != nil {
			return nil, nil, totalRows, time.Since(startTime), fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a map for this row
		rowMap := make(map[string]interface{})
		for i, col := range columnNames {
			val := columnValues[i]
			if val == nil {
				rowMap[col] = nil
			} else {
				rowMap[col] = r.convertValue(val, columnTypes[i])
			}

			// Add sample value to column metadata (from first row only)
			if rowCount == 0 && len(columns) > i {
				columns[i].SampleValue = rowMap[col]
			}
		}

		results = append(results, rowMap)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, nil, totalRows, time.Since(startTime), fmt.Errorf("row iteration error: %w", err)
	}

	executionTime = time.Since(startTime)

	// If we didn't get total count earlier and no pagination, use current count
	if totalRows == 0 && page == 0 {
		totalRows = int64(rowCount)
	}

	r.logger.WithFields(logrus.Fields{
		"query":          paginatedQuery,
		"row_count":      rowCount,
		"total_rows":     totalRows,
		"execution_time": executionTime,
		"page":           page,
		"page_size":      pageSize,
	}).Info("Query executed successfully with metadata")

	return results, columns, totalRows, executionTime, nil
}

// extractColumnMetadata extracts metadata about the result columns
func (r *QueryRepository) extractColumnMetadata(rows *sql.Rows) ([]models.ColumnMetadata, error) {
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}

	columns := make([]models.ColumnMetadata, len(columnTypes))
	for i, colType := range columnTypes {
		nullable, _ := colType.Nullable()

		columns[i] = models.ColumnMetadata{
			Name:         colType.Name(),
			Type:         r.simplifyTypeName(colType.DatabaseTypeName()),
			DatabaseType: colType.DatabaseTypeName(),
			Nullable:     nullable,
			Position:     i,
		}
	}

	return columns, nil
}

// simplifyTypeName converts database type names to simpler forms
func (r *QueryRepository) simplifyTypeName(dbType string) string {
	// Remove Nullable wrapper
	dbType = strings.TrimPrefix(dbType, "Nullable(")
	dbType = strings.TrimSuffix(dbType, ")")

	// Simplify common types
	switch {
	case strings.HasPrefix(dbType, "UInt"):
		return "UInt"
	case strings.HasPrefix(dbType, "Int"):
		return "Int"
	case strings.HasPrefix(dbType, "Float"):
		return "Float"
	case strings.HasPrefix(dbType, "Decimal"):
		return "Decimal"
	case strings.Contains(dbType, "String"):
		return "String"
	case strings.Contains(dbType, "Date"):
		return "Date"
	case strings.Contains(dbType, "DateTime"):
		return "DateTime"
	case strings.Contains(dbType, "UUID"):
		return "UUID"
	case strings.Contains(dbType, "Array"):
		return "Array"
	default:
		return dbType
	}
}

// getQueryTotalCount wraps the query in a count to get total rows
func (r *QueryRepository) getQueryTotalCount(ctx context.Context, query string) (int64, error) {
	// This is disabled for now as it can cause performance issues
	// In production, consider using EXPLAIN or approximate counts
	return -1, fmt.Errorf("total count disabled for performance")

	/*
		// Remove LIMIT clause if present for counting
		cleanQuery := r.removeLimitClause(query)

		// Wrap in COUNT query
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS subquery", cleanQuery)

		r.logger.WithField("count_query", countQuery).Debug("Getting total count")

		db := r.client.GetDB()
		var count int64

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		err := db.QueryRowContext(ctx, countQuery).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("failed to get total count: %w", err)
		}

		return count, nil
	*/
}

// removeLimitClause removes LIMIT clause from query
func (r *QueryRepository) removeLimitClause(query string) string {
	// Simple regex to remove LIMIT clause
	// This is a simplified version - for production, use a proper SQL parser
	upperQuery := strings.ToUpper(query)
	limitIndex := strings.LastIndex(upperQuery, "LIMIT")

	if limitIndex > 0 {
		// Check if this is actually a LIMIT clause (not in a string or comment)
		beforeLimit := query[:limitIndex]
		// Remove trailing whitespace and semicolon
		beforeLimit = strings.TrimSpace(beforeLimit)
		beforeLimit = strings.TrimSuffix(beforeLimit, ";")
		return beforeLimit
	}

	return strings.TrimSuffix(strings.TrimSpace(query), ";")
}

// addPagination adds LIMIT and OFFSET to a query for pagination
func (r *QueryRepository) addPagination(query string, page, pageSize int) string {
	// If no pagination requested (page=0 or pageSize=0), return query as-is
	if page <= 0 || pageSize <= 0 {
		return query
	}

	// Remove existing LIMIT clause if present
	query = r.removeLimitClause(query)

	// Calculate offset
	offset := (page - 1) * pageSize

	// Add pagination
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", query, pageSize, offset)
}

// ExecuteQuery - Legacy method for backward compatibility
func (r *QueryRepository) ExecuteQuery(ctx context.Context, query string) ([]map[string]interface{}, int, time.Duration, error) {
	results, _, _, executionTime, err := r.ExecuteQueryWithMetadata(ctx, query, 0, 0)
	if err != nil {
		return nil, 0, executionTime, err
	}
	return results, len(results), executionTime, nil
}

// GetQueryStatistics gets execution statistics for a query
func (r *QueryRepository) GetQueryStatistics(ctx context.Context, query string) (*models.QueryStatistics, error) {
	// Get query execution plan with statistics
	explainQuery := fmt.Sprintf("EXPLAIN ESTIMATE %s", query)

	db := r.client.GetDB()
	rows, err := db.QueryContext(ctx, explainQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get query statistics: %w", err)
	}
	defer rows.Close()

	stats := &models.QueryStatistics{}

	// Parse the EXPLAIN output to extract statistics
	// This is simplified - actual implementation would parse the output properly
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			continue
		}

		// Parse estimated rows, bytes, etc. from the explain output
		if strings.Contains(line, "rows") {
			// Extract row count estimation
		}
		if strings.Contains(line, "bytes") {
			// Extract bytes estimation
		}
	}

	return stats, nil
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
	return strings.Contains(s, substr)
}
