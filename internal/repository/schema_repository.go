package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/clickhouse"
	"gitlab.smartbet.am/golang/query-assistant/internal/models"
)

type SchemaRepository struct {
	client *clickhouse.Client
	logger *logrus.Logger
}

func NewSchemaRepository(client *clickhouse.Client, logger *logrus.Logger) *SchemaRepository {
	return &SchemaRepository{
		client: client,
		logger: logger,
	}
}

// GetDatabaseSchema retrieves the complete database schema
func (r *SchemaRepository) GetDatabaseSchema(ctx context.Context) (*models.SchemaInfo, error) {
	// Get current database name - use the native connection instead of SQL
	conn := r.client.GetConn()

	// Execute query using native connection
	rows, err := conn.Query(ctx, "SELECT currentDatabase()")
	if err != nil {
		return nil, fmt.Errorf("failed to get current database: %w", err)
	}
	defer rows.Close()

	var database string
	if rows.Next() {
		if err := rows.Scan(&database); err != nil {
			return nil, fmt.Errorf("failed to scan database name: %w", err)
		}
	}

	// Get all tables
	tables, err := r.getTables(ctx, database)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}

	// Get columns for each table
	for i := range tables {
		columns, err := r.getTableColumns(ctx, database, tables[i].Name)
		if err != nil {
			r.logger.WithError(err).WithField("table", tables[i].Name).Error("Failed to get columns")
			continue
		}
		tables[i].Columns = columns

		// Get row count estimate
		rowCount, err := r.getTableRowCount(ctx, database, tables[i].Name)
		if err != nil {
			r.logger.WithError(err).WithField("table", tables[i].Name).Warn("Failed to get row count")
			rowCount = 0
		}
		tables[i].RowCount = rowCount
	}

	return &models.SchemaInfo{
		Database: database,
		Tables:   tables,
	}, nil
}

// getTables retrieves all tables in the database
func (r *SchemaRepository) getTables(ctx context.Context, database string) ([]models.TableSchema, error) {
	query := `
		SELECT 
			name,
			engine,
			comment
		FROM system.tables
		WHERE database = ? AND engine NOT IN ('View', 'MaterializedView', 'Dictionary')
		ORDER BY name
	`

	// Use native connection
	conn := r.client.GetConn()
	rows, err := conn.Query(ctx, query, database)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []models.TableSchema
	for rows.Next() {
		var table models.TableSchema
		var comment sql.NullString

		if err := rows.Scan(&table.Name, &table.Engine, &comment); err != nil {
			return nil, err
		}

		if comment.Valid {
			table.Description = comment.String
		}

		tables = append(tables, table)
	}

	return tables, nil
}

// getTableColumns retrieves columns for a specific table
func (r *SchemaRepository) getTableColumns(ctx context.Context, database, table string) ([]models.ColumnSchema, error) {
	query := `
		SELECT 
			name,
			type,
			comment,
			is_in_primary_key,
			is_in_sorting_key,
			is_in_partition_key
		FROM system.columns
		WHERE database = ? AND table = ?
		ORDER BY position
	`

	// Use native connection
	conn := r.client.GetConn()
	rows, err := conn.Query(ctx, query, database, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []models.ColumnSchema
	for rows.Next() {
		var col models.ColumnSchema
		var comment sql.NullString
		var isPrimary, isSorting, isPartition uint8

		if err := rows.Scan(&col.Name, &col.Type, &comment, &isPrimary, &isSorting, &isPartition); err != nil {
			return nil, err
		}

		if comment.Valid {
			col.Description = comment.String
		}

		// Add key information to description
		var keyInfo []string
		if isPrimary == 1 {
			keyInfo = append(keyInfo, "PRIMARY KEY")
		}
		if isSorting == 1 {
			keyInfo = append(keyInfo, "SORTING KEY")
		}
		if isPartition == 1 {
			keyInfo = append(keyInfo, "PARTITION KEY")
		}

		if len(keyInfo) > 0 && col.Description != "" {
			col.Description += " [" + string(keyInfo[0])
			for i := 1; i < len(keyInfo); i++ {
				col.Description += ", " + keyInfo[i]
			}
			col.Description += "]"
		} else if len(keyInfo) > 0 {
			col.Description = "[" + string(keyInfo[0])
			for i := 1; i < len(keyInfo); i++ {
				col.Description += ", " + keyInfo[i]
			}
			col.Description += "]"
		}

		// Check if nullable
		col.IsNullable = contains(col.Type, "Nullable")

		columns = append(columns, col)
	}

	return columns, nil
}

// getTableRowCount gets an approximate row count for a table
func (r *SchemaRepository) getTableRowCount(ctx context.Context, database, table string) (uint64, error) {
	query := `
		SELECT sum(rows) 
		FROM system.parts 
		WHERE database = ? AND table = ? AND active
	`

	// Use native connection
	conn := r.client.GetConn()
	rows, err := conn.Query(ctx, query, database, table)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count sql.NullInt64
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}

	if count.Valid {
		return uint64(count.Int64), nil
	}

	return 0, nil
}

// GetTableSchema retrieves schema for a specific table
func (r *SchemaRepository) GetTableSchema(ctx context.Context, tableName string) (*models.TableSchema, error) {
	// Get current database using native connection
	conn := r.client.GetConn()
	rows, err := conn.Query(ctx, "SELECT currentDatabase()")
	if err != nil {
		return nil, fmt.Errorf("failed to get current database: %w", err)
	}
	defer rows.Close()

	var database string
	if rows.Next() {
		if err := rows.Scan(&database); err != nil {
			return nil, fmt.Errorf("failed to scan database name: %w", err)
		}
	}

	// Get table info
	query := `
		SELECT 
			name,
			engine,
			comment
		FROM system.tables
		WHERE database = ? AND name = ?
	`

	var table models.TableSchema
	var comment sql.NullString

	rows2, err := conn.Query(ctx, query, database, tableName)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	if !rows2.Next() {
		return nil, fmt.Errorf("table %s not found", tableName)
	}

	if err := rows2.Scan(&table.Name, &table.Engine, &comment); err != nil {
		return nil, err
	}

	if comment.Valid {
		table.Description = comment.String
	}

	// Get columns
	columns, err := r.getTableColumns(ctx, database, tableName)
	if err != nil {
		return nil, err
	}
	table.Columns = columns

	// Get row count
	rowCount, err := r.getTableRowCount(ctx, database, tableName)
	if err != nil {
		r.logger.WithError(err).Warn("Failed to get row count")
		rowCount = 0
	}
	table.RowCount = rowCount

	return &table, nil
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
