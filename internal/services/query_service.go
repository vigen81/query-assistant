package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/logger"
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
	log := logger.WithQuery(queryID)

	log.Info("Processing query request", map[string]interface{}{
		"prompt": req.Prompt,
	})

	// Get database schema
	schemaInfo, err := s.schemaRepo.GetDatabaseSchema(ctx)
	if err != nil {
		log.Error("Failed to get database schema", err, map[string]interface{}{})
		return nil, fmt.Errorf("failed to get database schema: %w", err)
	}

	// Generate SQL query using OpenAI
	generatedSQL, err := s.openaiClient.GenerateQuery(ctx, req.Prompt, schemaInfo)
	if err != nil {
		log.Error("Failed to generate SQL query", err, map[string]interface{}{})
		return nil, fmt.Errorf("failed to generate SQL query: %w", err)
	}

	log.Info("SQL query generated", map[string]interface{}{
		"generated_sql": generatedSQL,
	})

	// Validate the generated query
	if err := s.validateQuery(generatedSQL); err != nil {
		log.Error("Query validation failed", err, map[string]interface{}{
			"generated_sql": generatedSQL,
		})
		return nil, fmt.Errorf("query validation failed: %w", err)
	}

	// Validate query syntax with ClickHouse
	if err := s.queryRepo.ValidateQuery(ctx, generatedSQL); err != nil {
		log.Error("ClickHouse query validation failed", err, map[string]interface{}{
			"generated_sql": generatedSQL,
		})
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
		log.Error("Query execution failed", err, map[string]interface{}{
			"generated_sql": generatedSQL,
		})
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	log.Info("Query executed successfully", map[string]interface{}{
		"row_count":      rowCount,
		"execution_time": executionTime,
	})

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

// validateQuery performs security validation on the generated query
func (s *QueryService) validateQuery(query string) error {
	upperQuery := strings.ToUpper(query)

	// Check for forbidden keywords
	forbiddenKeywords := []string{
		"DROP", "DELETE", "TRUNCATE", "ALTER", "CREATE",
		"INSERT", "UPDATE", "GRANT", "REVOKE", "ATTACH",
		"DETACH", "RENAME", "REPLACE", "OPTIMIZE",
	}

	for _, keyword := range forbiddenKeywords {
		if strings.Contains(upperQuery, keyword) {
			return fmt.Errorf("query contains forbidden operation: %s", keyword)
		}
	}

	// Ensure it's a SELECT query
	trimmedQuery := strings.TrimSpace(upperQuery)
	if !strings.HasPrefix(trimmedQuery, "SELECT") && !strings.HasPrefix(trimmedQuery, "WITH") {
		return fmt.Errorf("only SELECT queries are allowed")
	}

	// Check for multiple statements
	if strings.Count(query, ";") > 1 {
		return fmt.Errorf("multiple statements are not allowed")
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
