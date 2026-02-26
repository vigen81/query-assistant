package services

import (
	"context"
	"fmt"
	"math"
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

// ProcessQuery processes a natural language query request with enhanced features
func (s *QueryService) ProcessQuery(ctx context.Context, req *models.QueryRequest) (*models.QueryResponse, error) {
	queryID := uuid.New().String()

	// Set defaults for pagination
	page := req.Page
	pageSize := req.PageSize

	// If no pagination is specified (page=0 or pageSize=0), don't apply pagination
	// Let the generated SQL's LIMIT clause take effect
	applyPagination := page > 0 && pageSize > 0

	if applyPagination {
		// Validate and set limits for pagination
		if pageSize > 10000 {
			pageSize = 10000 // Max page size
		}
	} else {
		// No pagination requested, let the SQL LIMIT work
		page = 0
		pageSize = 0
	}

	if req.SiteID <= 0 {
		return nil, fmt.Errorf("site_id is required and must be a positive number")
	}

	s.logger.WithFields(logrus.Fields{
		"query_id":         queryID,
		"site_id":          req.SiteID,
		"prompt":           req.Prompt,
		"page":             page,
		"page_size":        pageSize,
		"apply_pagination": applyPagination,
	}).Info("Processing query request")

	// Generate SQL query using OpenAI with Semantic Dictionary
	// (schema context is now provided by the dictionary, not by live DB introspection)
	generatedSQL, err := s.openaiClient.GenerateQuery(ctx, req.Prompt, req.SiteID)
	if err != nil {
		s.logger.WithError(err).Error("Failed to generate SQL query")
		return nil, fmt.Errorf("failed to generate SQL query: %w", err)
	}

	// Clean up the generated SQL
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

	// Execute the query with metadata
	results, columns, totalRows, executionTime, err := s.queryRepo.ExecuteQueryWithMetadata(
		queryCtx,
		generatedSQL,
		page,
		pageSize,
	)
	if err != nil {
		s.logger.WithError(err).Error("Query execution failed")
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"query_id":       queryID,
		"row_count":      len(results),
		"total_rows":     totalRows,
		"execution_time": executionTime,
		"columns":        len(columns),
	}).Info("Query executed successfully")

	// Build response
	response := &models.QueryResponse{
		QueryID:       queryID,
		Prompt:        req.Prompt,
		GeneratedSQL:  generatedSQL,
		Results:       results,
		RowCount:      len(results), // For backward compatibility
		ExecutionTime: executionTime.Seconds(),
		Timestamp:     time.Now(),
	}

	// Add column metadata if requested
	if req.IncludeSchema || len(columns) > 0 {
		response.Columns = columns
	}

	// Add pagination info only if pagination was requested
	if applyPagination {
		// If totalRows is unknown (-1), we can't calculate total pages accurately
		totalPages := 0
		if totalRows > 0 {
			totalPages = int(math.Ceil(float64(totalRows) / float64(pageSize)))
		}

		nextPage := page + 1
		prevPage := page - 1

		// Assume there might be more pages if we got a full page of results
		hasNext := len(results) == pageSize
		if totalRows > 0 {
			hasNext = page < totalPages
		}

		response.Pagination = &models.PaginationInfo{
			Page:        page,
			PageSize:    pageSize,
			TotalPages:  totalPages,
			TotalRows:   totalRows,
			HasNext:     hasNext,
			HasPrevious: page > 1,
		}

		if response.Pagination.HasNext {
			response.Pagination.NextPage = &nextPage
		}
		if response.Pagination.HasPrevious {
			response.Pagination.PreviousPage = &prevPage
		}
	} else {
		// No pagination requested - the results are limited by the SQL LIMIT clause
		// Still provide basic pagination info for consistency
		response.Pagination = &models.PaginationInfo{
			Page:        1,
			PageSize:    len(results),
			TotalPages:  0,
			TotalRows:   -1,
			HasNext:     false, // We don't know without counting
			HasPrevious: false,
		}
	}

	// Add statistics if requested
	if req.IncludeStats {
		response.Statistics = &models.QueryStatistics{
			TotalRows:       totalRows,
			RowsReturned:    len(results),
			ExecutionTimeMs: float64(executionTime.Milliseconds()),
		}

		// Try to get more detailed statistics
		if stats, err := s.queryRepo.GetQueryStatistics(ctx, generatedSQL); err == nil && stats != nil {
			response.Statistics.BytesProcessed = stats.BytesProcessed
			response.Statistics.PartitionsAccessed = stats.PartitionsAccessed
			response.Statistics.CacheHit = stats.CacheHit
		}
	}

	// Add warnings if applicable
	response.Warnings = s.generateWarnings(generatedSQL, totalRows, executionTime)

	return response, nil
}

// generateWarnings generates helpful warnings about the query
func (s *QueryService) generateWarnings(query string, totalRows int64, executionTime time.Duration) []string {
	var warnings []string

	upperQuery := strings.ToUpper(query)

	// Warning for large result sets
	if totalRows > 10000 {
		warnings = append(warnings, fmt.Sprintf("Query returned %d rows. Consider adding filters to reduce result size.", totalRows))
	}

	// Warning for slow queries
	if executionTime > 10*time.Second {
		warnings = append(warnings, fmt.Sprintf("Query took %.2f seconds. Consider optimizing the query.", executionTime.Seconds()))
	}

	// Warning for SELECT *
	if strings.Contains(upperQuery, "SELECT *") {
		warnings = append(warnings, "Using SELECT * may retrieve unnecessary columns and impact performance.")
	}

	// Warning for missing WHERE clause in large tables
	if !strings.Contains(upperQuery, "WHERE") && !strings.Contains(upperQuery, "LIMIT") {
		warnings = append(warnings, "Query has no WHERE clause. This might scan entire table.")
	}

	// Warning for cross joins
	if strings.Contains(upperQuery, "CROSS JOIN") {
		warnings = append(warnings, "Query uses CROSS JOIN which can produce very large result sets.")
	}

	return warnings
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

	// Check for dangerous keyword combinations
	for _, keyword := range dangerousKeywords {
		if strings.Contains(cleanQuery, keyword) {
			return fmt.Errorf("query contains forbidden operation: %s", keyword)
		}
	}

	// Check for multiple statements
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
		Valid:         true,
		Errors:        []string{},
		Warnings:      []string{},
		Operations:    []string{},
		Tables:        []string{},
		Columns:       []string{},
		Optimizations: []string{},
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

	// Extract operations
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
	if strings.Contains(upperQuery, "WITH") {
		result.Operations = append(result.Operations, "CTE")
	}

	// Add warnings and optimizations
	if !strings.Contains(upperQuery, "LIMIT") {
		result.Warnings = append(result.Warnings, "Query has no LIMIT clause")
		result.Optimizations = append(result.Optimizations, "Consider adding LIMIT clause for better performance")
	}

	if strings.Contains(upperQuery, "SELECT *") {
		result.Warnings = append(result.Warnings, "Using SELECT * may retrieve unnecessary columns")
		result.Optimizations = append(result.Optimizations, "Specify only the columns you need instead of SELECT *")
	}

	if !strings.Contains(upperQuery, "WHERE") && strings.Contains(upperQuery, "FROM") {
		result.Warnings = append(result.Warnings, "No WHERE clause detected")
		result.Optimizations = append(result.Optimizations, "Add WHERE clause to filter data and improve performance")
	}

	// Estimate cost based on operations
	if len(result.Operations) > 3 || strings.Contains(upperQuery, "JOIN") {
		result.EstimatedCost = "medium to high"
	} else {
		result.EstimatedCost = "low"
	}

	return result, nil
}
