package models

import (
	"time"
)

// QueryRequest represents the incoming query request
type QueryRequest struct {
	Prompt  string                 `json:"prompt" example:"Show me the top 10 users by total purchase amount in the last 30 days"`
	Context map[string]interface{} `json:"context,omitempty" swaggertype:"object"`
	Timeout int                    `json:"timeout,omitempty" example:"30" minimum:"1" maximum:"300"`
}

// QueryResponse represents the response for a query request
type QueryResponse struct {
	QueryID       string                   `json:"query_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Prompt        string                   `json:"prompt" example:"Show me the top 10 users by total purchase amount"`
	GeneratedSQL  string                   `json:"generated_sql" example:"SELECT user_id, SUM(amount) as total FROM purchases WHERE date >= today() - 30 GROUP BY user_id ORDER BY total DESC LIMIT 10"`
	Results       []map[string]interface{} `json:"results" swaggertype:"array,object"`
	RowCount      int                      `json:"row_count" example:"10"`
	ExecutionTime float64                  `json:"execution_time" example:"0.234"`
	Timestamp     time.Time                `json:"timestamp" example:"2023-01-01T00:00:00Z"`
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
}

// ColumnSchema represents a column's schema
type ColumnSchema struct {
	Name        string `json:"name" example:"user_id"`
	Type        string `json:"type" example:"UInt64"`
	Description string `json:"description,omitempty" example:"Unique user identifier"`
	IsNullable  bool   `json:"is_nullable" example:"false"`
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
	Prompt        string    `json:"prompt" example:"Show total sales by month"`
	GeneratedSQL  string    `json:"generated_sql" example:"SELECT month, SUM(sales) FROM sales_table GROUP BY month"`
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
}
