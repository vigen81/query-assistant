package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/models"
	"gitlab.smartbet.am/golang/query-assistant/internal/repository"
)

type SchemaHandler struct {
	schemaRepo *repository.SchemaRepository
	logger     *logrus.Logger
}

func NewSchemaHandler(schemaRepo *repository.SchemaRepository, logger *logrus.Logger) *SchemaHandler {
	return &SchemaHandler{
		schemaRepo: schemaRepo,
		logger:     logger,
	}
}

// GetDatabaseSchema retrieves the complete database schema
// @Summary Get database schema
// @Description Get the complete database schema including all tables and columns
// @Tags schema
// @Produce json
// @Success 200 {object} models.SchemaInfo
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /schema [get]
func (h *SchemaHandler) GetDatabaseSchema(c *fiber.Ctx) error {
	schema, err := h.schemaRepo.GetDatabaseSchema(c.Context())
	if err != nil {
		h.logger.WithError(err).Error("Failed to get database schema")
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:     "Failed to retrieve database schema",
			Code:      "SCHEMA_ERROR",
			Timestamp: time.Now(),
		})
	}

	return c.JSON(schema)
}

// GetTableSchema retrieves schema for a specific table
// @Summary Get table schema
// @Description Get schema information for a specific table
// @Tags schema
// @Produce json
// @Param table path string true "Table name"
// @Success 200 {object} models.TableSchema
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /schema/table/{table} [get]
func (h *SchemaHandler) GetTableSchema(c *fiber.Ctx) error {
	tableName := c.Params("table")
	if tableName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Table name is required",
			Code:      "MISSING_TABLE",
			Timestamp: time.Now(),
		})
	}

	tableSchema, err := h.schemaRepo.GetTableSchema(c.Context(), tableName)
	if err != nil {
		if err.Error() == "table not found" {
			return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
				Error:     "Table not found",
				Code:      "TABLE_NOT_FOUND",
				Message:   "The specified table does not exist",
				Timestamp: time.Now(),
			})
		}

		h.logger.WithError(err).WithField("table", tableName).Error("Failed to get table schema")
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:     "Failed to retrieve table schema",
			Code:      "SCHEMA_ERROR",
			Timestamp: time.Now(),
		})
	}

	return c.JSON(tableSchema)
}

// RefreshSchema manually refreshes schema cache (if implemented)
// @Summary Refresh schema cache
// @Description Manually refresh the database schema cache
// @Tags schema
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /schema/refresh [post]
func (h *SchemaHandler) RefreshSchema(c *fiber.Ctx) error {
	// This would trigger a schema cache refresh if you implement caching
	h.logger.Info("Schema refresh requested")

	// For now, just return success
	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "Schema refresh initiated",
	})
}
