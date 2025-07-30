package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/logger"
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

// ExecuteQuery handles natural language query requests
// @Summary Execute a natural language query
// @Description Convert a natural language prompt to SQL and execute it against ClickHouse
// @Tags query
// @Accept json
// @Produce json
// @Param query body models.QueryRequest true "Query request"
// @Success 200 {object} models.QueryResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 408 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /query/execute [post]
func (h *QueryHandler) ExecuteQuery(c *fiber.Ctx) error {
	var req models.QueryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Invalid request body",
			Code:      "INVALID_REQUEST",
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

	// Get user ID from context (set by auth middleware)
	userID := ""
	if uid := c.Locals("user_id"); uid != nil {
		userID = uid.(string)
	}

	log := logger.WithUser(userID)
	log.Info("Processing query request", map[string]interface{}{
		"prompt": req.Prompt,
	})

	// Process the query
	response, err := h.queryService.ProcessQuery(c.Context(), &req)
	if err != nil {
		log.Error("Query processing failed", err, map[string]interface{}{
			"prompt": req.Prompt,
		})

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

	return c.JSON(response)
}

// ValidateQuery validates a SQL query without executing it
// @Summary Validate a SQL query
// @Description Validate a SQL query for syntax and security without executing it
// @Tags query
// @Accept json
// @Produce json
// @Param query body map[string]string true "Query to validate" example({"query": "SELECT * FROM users LIMIT 10"})
// @Success 200 {object} models.QueryValidationResult
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
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
// @Param request body map[string]string true "Generation request" example({"prompt": "Show me top 10 users by purchase amount"})
// @Success 200 {object} map[string]string
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
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

	// This would be a simplified version that only generates without executing
	// You might want to add a separate method in QueryService for this
	return c.JSON(fiber.Map{
		"message": "Query generation endpoint - to be implemented",
		"prompt":  req.Prompt,
	})
}
