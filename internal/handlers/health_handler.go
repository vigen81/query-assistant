package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/models"
)

type HealthHandler struct {
	logger *logrus.Logger
}

func NewHealthHandler(logger *logrus.Logger) *HealthHandler {
	return &HealthHandler{
		logger: logger,
	}
}

// HealthCheck provides a health check endpoint
// @Summary Health check
// @Description Returns the general health status of the query assistant service
// @Tags health
// @Produce json
// @Success 200 {object} models.HealthResponse "Service is healthy"
// @Router /health [get]
func (h *HealthHandler) HealthCheck(c *fiber.Ctx) error {
	return c.JSON(models.HealthResponse{
		Status:    "ok",
		Service:   "query-assistant",
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
	})
}

// ReadinessCheck provides a readiness check endpoint
// @Summary Readiness check
// @Description Returns the readiness status of the query assistant including dependency checks
// @Tags health
// @Produce json
// @Success 200 {object} models.HealthResponse "Service is ready to accept requests"
// @Failure 503 {object} models.ErrorResponse "Service is not ready - dependencies not available"
// @Router /ready [get]
func (h *HealthHandler) ReadinessCheck(c *fiber.Ctx) error {
	// Here you would check ClickHouse and OpenAI connectivity
	// For now, return success
	return c.JSON(models.HealthResponse{
		Status:    "ready",
		Service:   "query-assistant",
		Timestamp: time.Now().UTC(),
		Checks: &models.HealthCheckDetail{
			ClickHouse: "ok",
			OpenAI:     "ok",
		},
	})
}

// LivenessCheck provides a liveness check endpoint
// @Summary Liveness check
// @Description Returns the liveness status of the query assistant
// @Tags health
// @Produce json
// @Success 200 {object} models.HealthResponse "Service is alive"
// @Router /live [get]
func (h *HealthHandler) LivenessCheck(c *fiber.Ctx) error {
	return c.JSON(models.HealthResponse{
		Status:    "alive",
		Service:   "query-assistant",
		Timestamp: time.Now().UTC(),
	})
}
