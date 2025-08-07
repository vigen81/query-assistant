package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/models"
	"gitlab.smartbet.am/golang/query-assistant/internal/openai"
	"gitlab.smartbet.am/golang/query-assistant/internal/repository"
)

type QueryService struct {
	openaiClient *openai.Client
	schemaRepo   *repository.SchemaRepository
	queryRepo    *repository.QueryRepository
	logger       *logrus.Logger
}

func NewQueryService(
	openaiClient *openai.Client,
	schemaRepo *repository.SchemaRepository,
	queryRepo *repository.QueryRepository,
	logger *logrus.Logger,
) *QueryService {
	return &QueryService{
		openaiClient: openaiClient,
		schemaRepo:   schemaRepo,
		queryRepo:    queryRepo,
		logger:       logger,
	}
}

// ProcessQuery processes a natural language query request
func (s *QueryService) ProcessQuery(ctx context.Context, req *models.QueryRequest) (*models.QueryResponse, error) {
	queryID := uuid.New().String()

	s.logger.WithFields(logrus.Fields{
		"query_id": queryID,
		"prompt":   req.Prompt,
	}).Info("Processing query request")

	// Get database schema
	schemaInfo, err := s.schemaRepo.GetDatabaseSchema(ctx)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get database schema")
		return nil, fmt.Errorf("failed to get database schema: %w", err)
	}

	// Generate SQL query using OpenAI
	generatedSQL, err := s.openaiClient.GenerateQuery(ctx, req.Prompt, schemaInfo)
	if err != nil {
		s.logger.WithError(err).Error("Failed to generate SQL query")
		return nil, fmt.Errorf("failed to generate SQL query: %w", err)
	}

	// Clean up the generated SQL (remove markdown, extra text, etc.)
	generatedSQL = s.cleanGeneratedSQL(generatedSQL)

	s.logger.WithFields(logrus.Fields{
		"query_id":      queryID,
		"generated_sql": generatedSQL,
	}).Info("SQL query generated")

	// Validate the generated query
	if err := s.validateQuery(generatedSQL); err != nil {
		s.logger.WithFields(logrus.Fields{
			"query_id":      queryID,
			"generated_sql": generatedSQL,
			"error":         err.Error(),
		}).Error("Query validation failed")
		return nil, fmt.Errorf("query validation failed: %w", err)
	}

	// Validate query syntax with ClickHouse
	if err := s.queryRepo.ValidateQuery(ctx, generatedSQL); err != nil {
		s.logger.WithError(err).Error("ClickHouse query validation failed")
		return nil, fmt.Errorf("ClickHouse query validation failed: %w", err)
	}

	// Set query timeout
	timeout := 30 * time.Second
	if req.Timeout > 0 && req.Timeout <= 300 {
		timeout = time.Duration(req.Timeout) * time.Second
	}

	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute the query
	results, rowCount, executionTime, err := s.queryRepo.ExecuteQuery(queryCtx, generatedSQL)
	if err != nil {
		s.logger.WithError(err).Error("Query execution failed")
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"query_id":       queryID,
		"row_count":      rowCount,
		"execution_time": executionTime,
	}).Info("Query executed successfully")

	response := &models.QueryResponse{
		QueryID:       queryID,
		Prompt:        req.Prompt,
		GeneratedSQL:  generatedSQL,
		Results:       results,
		RowCount:      rowCount,
		ExecutionTime: executionTime.Seconds(),
		Timestamp:     time.Now(),
	}

	return response, nil
}

// cleanGeneratedSQL removes markdown formatting and extra text from generated SQL
func (s *QueryService) cleanGeneratedSQL(sql string) string {
	// Remove markdown code blocks
	sql = regexp.MustCompile("```sql\n?").ReplaceAllString(sql, "")
	sql = regexp.MustCompile("```\n?").ReplaceAllString(sql, "")

	// Remove any lines that start with explanation text
	lines := strings.Split(sql, "\n")
	var sqlLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines or lines that look like explanations
		if trimmed == "" || strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Stop if we hit explanation text after the query
		if strings.Contains(strings.ToLower(trimmed), "this query") ||
			strings.Contains(strings.ToLower(trimmed), "the above") ||
			strings.Contains(strings.ToLower(trimmed), "explanation:") {
			break
		}
		sqlLines = append(sqlLines, line)
	}

	sql = strings.Join(sqlLines, "\n")
	sql = strings.TrimSpace(sql)

	// Ensure it ends with semicolon
	if !strings.HasSuffix(sql, ";") {
		sql += ";"
	}

	return sql
}

// validateQuery performs security validation on the generated query
func (s *QueryService) validateQuery(query string) error {
	upperQuery := strings.ToUpper(query)

	// Remove comments for validation
	cleanQuery := regexp.MustCompile(`--.*$`).ReplaceAllString(upperQuery, "")
	cleanQuery = regexp.MustCompile(`/\*.*?\*/`).ReplaceAllString(cleanQuery, "")

	// Check if it's fundamentally a SELECT query (WITH is allowed for CTEs)
	trimmedQuery := strings.TrimSpace(cleanQuery)
	if !strings.HasPrefix(trimmedQuery, "SELECT") && !strings.HasPrefix(trimmedQuery, "WITH") {
		return fmt.Errorf("only SELECT queries are allowed")
	}

	// List of truly dangerous keywords that should never appear
	dangerousKeywords := []string{
		"DROP TABLE", "DROP DATABASE", "DROP VIEW",
		"DELETE FROM", "TRUNCATE TABLE",
		"ALTER TABLE", "ALTER DATABASE",
		"CREATE TABLE", "CREATE DATABASE", "CREATE VIEW",
		"INSERT INTO", "UPDATE SET",
		"GRANT", "REVOKE",
		"ATTACH", "DETACH",
		"RENAME", "REPLACE",
		"OPTIMIZE TABLE",
		"KILL", "SYSTEM",
	}

	// Check for dangerous keyword combinations (not just individual words)
	for _, keyword := range dangerousKeywords {
		if strings.Contains(cleanQuery, keyword) {
			return fmt.Errorf("query contains forbidden operation: %s", keyword)
		}
	}

	// Check for multiple statements (but allow semicolon at the end)
	queryWithoutTrailingSemi := strings.TrimSuffix(strings.TrimSpace(query), ";")
	if strings.Count(queryWithoutTrailingSemi, ";") > 0 {
		return fmt.Errorf("multiple statements are not allowed")
	}

	// Additional checks for suspicious patterns
	suspiciousPatterns := []string{
		"INTO OUTFILE",
		"INTO DUMPFILE",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(cleanQuery, pattern) {
			return fmt.Errorf("query contains suspicious pattern: %s", pattern)
		}
	}

	return nil
}

// GetQueryValidation validates a query without executing it
func (s *QueryService) GetQueryValidation(ctx context.Context, query string) (*models.QueryValidationResult, error) {
	result := &models.QueryValidationResult{
		Valid:      true,
		Errors:     []string{},
		Warnings:   []string{},
		Operations: []string{},
		Tables:     []string{},
		Columns:    []string{},
	}

	// First, do basic validation
	if err := s.validateQuery(query); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err.Error())
		return result, nil
	}

	// Then validate with ClickHouse
	if err := s.queryRepo.ValidateQuery(ctx, query); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("ClickHouse validation: %s", err.Error()))
		return result, nil
	}

	// Extract operations (simplified - you might want to use a proper SQL parser)
	upperQuery := strings.ToUpper(query)
	if strings.Contains(upperQuery, "SELECT") {
		result.Operations = append(result.Operations, "SELECT")
	}
	if strings.Contains(upperQuery, "JOIN") {
		result.Operations = append(result.Operations, "JOIN")
	}
	if strings.Contains(upperQuery, "GROUP BY") {
		result.Operations = append(result.Operations, "GROUP BY")
	}
	if strings.Contains(upperQuery, "ORDER BY") {
		result.Operations = append(result.Operations, "ORDER BY")
	}

	// Add warnings for potentially expensive operations
	if !strings.Contains(upperQuery, "LIMIT") {
		result.Warnings = append(result.Warnings, "Query has no LIMIT clause - consider adding one for better performance")
	}

	if strings.Contains(upperQuery, "SELECT *") {
		result.Warnings = append(result.Warnings, "Using SELECT * may retrieve unnecessary columns")
	}

	return result, nil
}
