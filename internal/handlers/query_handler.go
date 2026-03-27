package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/models"
	"gitlab.smartbet.am/golang/query-assistant/internal/services"
)

type QueryHandler struct {
	queryService *services.QueryService
	logger       *logrus.Logger
}

func NewQueryHandler(queryService *services.QueryService, logger *logrus.Logger) *QueryHandler {
	return &QueryHandler{
		queryService: queryService,
		logger:       logger,
	}
}

// ExecuteQuery handles natural language query requests with multi-tenant filtering
// @Summary Execute a natural language query with site filtering
// @Description Convert a natural language prompt to SQL and execute it against ClickHouse with site_id filtering for multi-tenancy
// @Tags query
// @Accept json
// @Produce json
// @Param query body models.QueryRequest true "Query request with required site_id for multi-tenant filtering"
// @Success 200 {object} models.QueryResponse "Successful query execution with results filtered by site_id"
// @Failure 400 {object} models.ErrorResponse "Invalid request - missing site_id or invalid format"
// @Failure 401 {object} models.ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 408 {object} models.ErrorResponse "Query timeout"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /query/execute [post]
func (h *QueryHandler) ExecuteQuery(c *fiber.Ctx) error {
	var req models.QueryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Invalid request body",
			Code:      "INVALID_REQUEST",
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	// Validate request
	if strings.TrimSpace(req.Prompt) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Prompt is required",
			Code:      "MISSING_PROMPT",
			Timestamp: time.Now(),
		})
	}

	if req.SiteID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Site ID is required",
			Code:      "MISSING_SITE_ID",
			Message:   "site_id parameter is required for multi-tenant filtering",
			Timestamp: time.Now(),
		})
	}

	// Validate pagination parameters
	if req.Page < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Invalid page number",
			Code:      "INVALID_PAGE",
			Message:   "Page number must be 0 (no pagination) or greater than 0",
			Timestamp: time.Now(),
		})
	}

	if req.PageSize < 0 || req.PageSize > 10000 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Invalid page size",
			Code:      "INVALID_PAGE_SIZE",
			Message:   "Page size must be between 1 and 10000",
			Timestamp: time.Now(),
		})
	}

	// Set default for include_schema if not specified
	if !req.IncludeSchema {
		req.IncludeSchema = true // Default to including column metadata
	}

	h.logger.WithFields(logrus.Fields{
		"prompt":         req.Prompt,
		"site_id":        req.SiteID,
		"page":           req.Page,
		"page_size":      req.PageSize,
		"include_schema": req.IncludeSchema,
		"include_stats":  req.IncludeStats,
	}).Info("Processing query request")

	// Process the query
	response, err := h.queryService.ProcessQuery(c.Context(), &req)
	if err != nil {
		h.logger.WithError(err).WithFields(logrus.Fields{"prompt": req.Prompt, "site_id": req.SiteID}).Error("Query processing failed")

		// Determine appropriate error code
		code := "QUERY_ERROR"
		status := fiber.StatusInternalServerError

		if strings.Contains(err.Error(), "validation failed") {
			code = "VALIDATION_ERROR"
			status = fiber.StatusBadRequest
		} else if strings.Contains(err.Error(), "timeout") {
			code = "TIMEOUT_ERROR"
			status = fiber.StatusRequestTimeout
		} else if strings.Contains(err.Error(), "generate") {
			code = "GENERATION_ERROR"
		}

		return c.Status(status).JSON(models.ErrorResponse{
			Error:     "Query processing failed",
			Code:      code,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	// Add response headers for pagination
	if response.Pagination != nil {
		c.Set("X-Total-Count", fmt.Sprintf("%d", response.Pagination.TotalRows))
		c.Set("X-Page", fmt.Sprintf("%d", response.Pagination.Page))
		c.Set("X-Page-Size", fmt.Sprintf("%d", response.Pagination.PageSize))
		c.Set("X-Total-Pages", fmt.Sprintf("%d", response.Pagination.TotalPages))
	}

	return c.JSON(response)
}

// ValidateQuery validates a SQL query without executing it
// @Summary Validate a SQL query
// @Description Validate a SQL query for syntax and security without executing it, with optimization suggestions
// @Tags query
// @Accept json
// @Produce json
// @Param query body models.QueryValidateRequest true "Query to validate with site_id"
// @Success 200 {object} models.QueryValidationResult "Validation result with warnings and optimization suggestions"
// @Failure 400 {object} models.ErrorResponse "Invalid request"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /query/validate [post]
func (h *QueryHandler) ValidateQuery(c *fiber.Ctx) error {
	var req struct {
		Query string `json:"query"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Invalid request body",
			Code:      "INVALID_REQUEST",
			Timestamp: time.Now(),
		})
	}

	if strings.TrimSpace(req.Query) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Query is required",
			Code:      "MISSING_QUERY",
			Timestamp: time.Now(),
		})
	}

	result, err := h.queryService.GetQueryValidation(c.Context(), req.Query)
	if err != nil {
		h.logger.WithError(err).Error("Failed to validate query")
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:     "Query validation failed",
			Code:      "VALIDATION_ERROR",
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	return c.JSON(result)
}

// GenerateQuery generates SQL from natural language without executing
// @Summary Generate SQL query
// @Description Generate a SQL query from natural language prompt without executing it
// @Tags query
// @Accept json
// @Produce json
// @Param request body models.QueryGenerateRequest true "Generation request with site_id"
// @Success 200 {object} models.QueryGenerateResponse "Generated SQL query with metadata"
// @Failure 400 {object} models.ErrorResponse "Invalid request"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /query/generate [post]
func (h *QueryHandler) GenerateQuery(c *fiber.Ctx) error {
	var req struct {
		Prompt string `json:"prompt"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Invalid request body",
			Code:      "INVALID_REQUEST",
			Timestamp: time.Now(),
		})
	}

	if strings.TrimSpace(req.Prompt) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Prompt is required",
			Code:      "MISSING_PROMPT",
			Timestamp: time.Now(),
		})
	}

	// For now, return a placeholder
	// In a production system, you'd implement a GenerateOnly method in QueryService
	// that would use: &models.QueryRequest{Prompt: req.Prompt, Timeout: 10}
	return c.JSON(fiber.Map{
		"prompt":        req.Prompt,
		"generated_sql": "-- SQL generation only endpoint to be implemented",
		"message":       "This endpoint would generate SQL without executing it",
		"timestamp":     time.Now(),
	})
}

// QueryHistory returns the history of executed queries
// @Summary Get query history
// @Description Retrieve the history of previously executed queries with pagination
// @Tags query
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(50)
// @Param user_id query string false "Filter by user ID"
// @Param site_id query int false "Filter by site ID"
// @Success 200 {array} models.QueryHistory "List of historical queries"
// @Failure 401 {object} models.ErrorResponse "Unauthorized"
// @Failure 500 {object} models.ErrorResponse "Internal server error"
// @Router /query/history [get]
func (h *QueryHandler) QueryHistory(c *fiber.Ctx) error {
	// This would be implemented with a proper query history storage
	return c.JSON(fiber.Map{
		"message": "Query history endpoint - to be implemented",
		"info":    "This would return paginated query history",
	})
}
