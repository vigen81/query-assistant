package models

import (
	"time"
)

// QueryRequest represents the incoming query request with multi-tenant support
type QueryRequest struct {
	Prompt  string                 `json:"prompt" example:"Show me the top 10 users by total purchase amount in the last 30 days"`
	SiteID  int64                  `json:"site_id" validate:"required,min=1" example:"123"` // Required site_id parameter (numeric)
	Context map[string]interface{} `json:"context,omitempty" swaggertype:"object"`
	Timeout int                    `json:"timeout,omitempty" example:"30" minimum:"1" maximum:"300"`
	// Pagination options
	Page     int `json:"page,omitempty" example:"1" minimum:"1"`
	PageSize int `json:"page_size,omitempty" example:"100" minimum:"1" maximum:"10000"`
	// Response options
	IncludeSchema bool `json:"include_schema,omitempty" example:"true"`
	IncludeStats  bool `json:"include_stats,omitempty" example:"true"`
}

// QueryValidateRequest represents a query validation request
type QueryValidateRequest struct {
	Query  string `json:"query" validate:"required" example:"SELECT * FROM users WHERE site_id = 123 LIMIT 10"`
	SiteID int64  `json:"site_id" validate:"required,min=1" example:"123"`
}

// QueryGenerateRequest represents a query generation request
type QueryGenerateRequest struct {
	Prompt string `json:"prompt" validate:"required" example:"Show GGR for last month"`
	SiteID int64  `json:"site_id" validate:"required,min=1" example:"123"`
}

// QueryGenerateResponse represents the response from query generation
type QueryGenerateResponse struct {
	Prompt       string    `json:"prompt" example:"Show GGR for last month"`
	GeneratedSQL string    `json:"generated_sql" example:"SELECT SUM(bet_amount) - SUM(win_amount) as GGR FROM transactions WHERE site_id = 123 AND created_at >= today() - 30 LIMIT 1"`
	SiteID       int64     `json:"site_id" example:"123"`
	Timestamp    time.Time `json:"timestamp" example:"2023-01-01T00:00:00Z"`
	Complexity   string    `json:"complexity,omitempty" example:"medium"`
}

// ColumnMetadata represents metadata about a result column
type ColumnMetadata struct {
	Name         string      `json:"name" example:"user_id"`
	Type         string      `json:"type" example:"UInt64"`
	DatabaseType string      `json:"database_type" example:"UInt64"`
	Nullable     bool        `json:"nullable" example:"false"`
	Position     int         `json:"position" example:"0"`
	SampleValue  interface{} `json:"sample_value,omitempty" swaggertype:"string" example:"12345"`
}

// QueryStatistics represents statistics about the query execution
type QueryStatistics struct {
	TotalRows          int64   `json:"total_rows" example:"50000"`
	RowsReturned       int     `json:"rows_returned" example:"100"`
	BytesProcessed     int64   `json:"bytes_processed,omitempty" example:"1048576"`
	ExecutionTimeMs    float64 `json:"execution_time_ms" example:"234.5"`
	CacheHit           bool    `json:"cache_hit,omitempty" example:"false"`
	PartitionsAccessed int     `json:"partitions_accessed,omitempty" example:"3"`
}

// PaginationInfo represents pagination metadata
type PaginationInfo struct {
	Page         int   `json:"page" example:"1"`
	PageSize     int   `json:"page_size" example:"100"`
	TotalPages   int   `json:"total_pages" example:"10"`
	TotalRows    int64 `json:"total_rows" example:"1000"`
	HasNext      bool  `json:"has_next" example:"true"`
	HasPrevious  bool  `json:"has_previous" example:"false"`
	NextPage     *int  `json:"next_page,omitempty" swaggertype:"integer" example:"2"`
	PreviousPage *int  `json:"previous_page,omitempty" swaggertype:"integer"`
}

// QueryResponse represents the enhanced response for a query request
type QueryResponse struct {
	QueryID      string                   `json:"query_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Prompt       string                   `json:"prompt" example:"Show me the top 10 users by total purchase amount"`
	GeneratedSQL string                   `json:"generated_sql" example:"SELECT user_id, SUM(amount) as total FROM purchases WHERE site_id = 123 AND created_at >= today() - 30 GROUP BY user_id ORDER BY total DESC LIMIT 10"`
	Results      []map[string]interface{} `json:"results" swaggertype:"array,object"`
	// New fields for column metadata and pagination
	Columns    []ColumnMetadata `json:"columns,omitempty"`
	Statistics *QueryStatistics `json:"statistics,omitempty"`
	Pagination *PaginationInfo  `json:"pagination,omitempty"`
	// Legacy fields (kept for backward compatibility)
	RowCount      int       `json:"row_count" example:"10"`
	ExecutionTime float64   `json:"execution_time" example:"0.234"`
	Timestamp     time.Time `json:"timestamp" example:"2023-01-01T00:00:00Z"`
	// Additional metadata
	Warnings []string `json:"warnings,omitempty" example:"['Query processed large amount of data']"`
	CacheKey string   `json:"cache_key,omitempty"`
}

// SchemaInfo represents database schema information
type SchemaInfo struct {
	Database string        `json:"database" example:"analytics"`
	Tables   []TableSchema `json:"tables"`
}

// TableSchema represents a single table's schema
type TableSchema struct {
	Name        string         `json:"name" example:"users"`
	Engine      string         `json:"engine" example:"MergeTree"`
	RowCount    uint64         `json:"row_count" example:"1000000"`
	Columns     []ColumnSchema `json:"columns"`
	Description string         `json:"description,omitempty" example:"User information table"`
	// New fields
	PartitionKey string     `json:"partition_key,omitempty" example:"toYYYYMM(created_at)"`
	SortingKey   string     `json:"sorting_key,omitempty" example:"user_id"`
	SizeBytes    int64      `json:"size_bytes,omitempty" example:"104857600"`
	LastModified *time.Time `json:"last_modified,omitempty"`
}

// ColumnSchema represents a column's schema
type ColumnSchema struct {
	Name        string `json:"name" example:"user_id"`
	Type        string `json:"type" example:"UInt64"`
	Description string `json:"description,omitempty" example:"Unique user identifier"`
	IsNullable  bool   `json:"is_nullable" example:"false"`
	// New fields
	DefaultValue   string `json:"default_value,omitempty"`
	IsPrimaryKey   bool   `json:"is_primary_key,omitempty" example:"true"`
	IsSortingKey   bool   `json:"is_sorting_key,omitempty" example:"true"`
	IsPartitionKey bool   `json:"is_partition_key,omitempty" example:"false"`
}

// ErrorResponse represents error responses
type ErrorResponse struct {
	Error     string      `json:"error" example:"Query validation failed"`
	Code      string      `json:"code" example:"VALIDATION_ERROR"`
	Message   string      `json:"message,omitempty" example:"Query contains forbidden operation: DROP"`
	QueryID   string      `json:"query_id,omitempty" example:"550e8400-e29b-41d4-a716-446655440000"`
	Timestamp time.Time   `json:"timestamp" example:"2023-01-01T00:00:00Z"`
	Details   interface{} `json:"details,omitempty" swaggertype:"object"`
}

// HealthResponse represents health check responses
type HealthResponse struct {
	Status    string             `json:"status" example:"ok"`
	Service   string             `json:"service" example:"query-assistant"`
	Timestamp time.Time          `json:"timestamp" example:"2023-01-01T00:00:00Z"`
	Version   string             `json:"version,omitempty" example:"1.0.0"`
	Checks    *HealthCheckDetail `json:"checks,omitempty"`
}

// HealthCheckDetail represents detailed health check information
type HealthCheckDetail struct {
	ClickHouse string `json:"clickhouse" example:"ok"`
	OpenAI     string `json:"openai" example:"ok"`
}

// QueryHistory represents a historical query execution
type QueryHistory struct {
	QueryID       string    `json:"query_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	UserID        string    `json:"user_id" example:"user123"`
	SiteID        int64     `json:"site_id" example:"123"`
	Prompt        string    `json:"prompt" example:"Show total sales by month"`
	GeneratedSQL  string    `json:"generated_sql" example:"SELECT month, SUM(sales) FROM sales_table WHERE site_id = 123 GROUP BY month"`
	Status        string    `json:"status" example:"success"`
	RowCount      int       `json:"row_count" example:"12"`
	ExecutionTime float64   `json:"execution_time" example:"0.156"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"created_at" example:"2023-01-01T00:00:00Z"`
}

// OpenAIMessage represents a message in the OpenAI chat format
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// QueryValidationResult represents the result of query validation
type QueryValidationResult struct {
	Valid      bool     `json:"valid"`
	Errors     []string `json:"errors,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
	Operations []string `json:"operations,omitempty"`
	Tables     []string `json:"tables,omitempty"`
	Columns    []string `json:"columns,omitempty"`
	// New fields
	EstimatedRows int64    `json:"estimated_rows,omitempty" example:"10000"`
	EstimatedCost string   `json:"estimated_cost,omitempty" example:"low"`
	Optimizations []string `json:"optimizations,omitempty"`
}
