package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/models"
	"gitlab.smartbet.am/golang/query-assistant/internal/services"
)

// BannerHandler exposes the banner image generation endpoints.
type BannerHandler struct {
	bannerService *services.BannerService
	logger        *logrus.Logger
}

// NewBannerHandler creates a BannerHandler.
func NewBannerHandler(bannerService *services.BannerService, logger *logrus.Logger) *BannerHandler {
	return &BannerHandler{
		bannerService: bannerService,
		logger:        logger,
	}
}

// Generate accepts a banner generation request and returns a generation ID.
//
// @Summary      Start async banner image generation
// @Description  Accepts a banner generation request and returns a generation ID immediately.
//
//	Poll GET /banner/generate/:id for status and results.
//
// @Tags         banner
// @Accept       json
// @Produce      json
// @Param        Authorization  header    string                          true  "Bearer token"
// @Param        request        body      models.BannerGenerationRequest  true  "Generation parameters"
// @Success      202            {object}  models.BannerGenerationAccepted
// @Failure      400            {object}  models.ErrorResponse
// @Failure      500            {object}  models.ErrorResponse
// @Security     ApiKeyAuth
// @Router       /banner/generate [post]
func (h *BannerHandler) Generate(c *fiber.Ctx) error {
	var req models.BannerGenerationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Invalid request body",
			Code:      "INVALID_REQUEST",
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	accepted, err := h.bannerService.Generate(c.Context(), &req)
	if err != nil {
		h.logger.WithError(err).Warn("Banner generation request rejected")
		status := fiber.StatusBadRequest
		code := "VALIDATION_ERROR"
		// Only promotion errors that aren't user input errors get 500.
		return c.Status(status).JSON(models.ErrorResponse{
			Error:     "Banner generation request failed",
			Code:      code,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	return c.Status(fiber.StatusAccepted).JSON(accepted)
}

// GetStatus returns the current state (and results) of a generation job.
//
// @Summary      Get banner generation status
// @Description  Returns status, variants (URLs / b64) and failure info for a generation job.
// @Tags         banner
// @Produce      json
// @Param        Authorization  header    string  true  "Bearer token"
// @Param        id             path      string  true  "Generation ID"
// @Success      200            {object}  models.BannerGenerationResult
// @Failure      400            {object}  models.ErrorResponse
// @Failure      404            {object}  models.ErrorResponse
// @Security     ApiKeyAuth
// @Router       /banner/generate/{id} [get]
func (h *BannerHandler) GetStatus(c *fiber.Ctx) error {
	generationID := c.Params("id")
	if generationID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:     "Generation ID is required",
			Code:      "MISSING_ID",
			Timestamp: time.Now(),
		})
	}

	result, err := h.bannerService.GetStatus(generationID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:     "Generation not found",
			Code:      "NOT_FOUND",
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	return c.JSON(result)
}
