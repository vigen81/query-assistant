package handlers

import (
	"encoding/base64"
	"fmt"
	"strconv"
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
// @Description  Accepts a banner generation request and returns a generation ID immediately (HTTP 202).
// @Description  Poll GET /banner/generate/{id} for status and results.
// @Description  gpt-image-1 returns b64_json by default — the URL field may be empty.
// @Tags         banner
// @Accept       json
// @Produce      json
// @Param        request  body      models.BannerGenerationRequest  true  "Generation parameters"
// @Success      202      {object}  models.BannerGenerationAccepted
// @Failure      400      {object}  models.BannerErrorResponse
// @Failure      500      {object}  models.BannerErrorResponse
// @Router       /banner/generate [post]
func (h *BannerHandler) Generate(c *fiber.Ctx) error {
	var req models.BannerGenerationRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.BannerErrorResponse{
			Error:     "Invalid request body",
			Code:      "INVALID_REQUEST",
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	accepted, err := h.bannerService.Generate(c.Context(), &req)
	if err != nil {
		h.logger.WithError(err).Warn("Banner generation request rejected")
		return c.Status(fiber.StatusBadRequest).JSON(models.BannerErrorResponse{
			Error:     "Banner generation request failed",
			Code:      "VALIDATION_ERROR",
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	return c.Status(fiber.StatusAccepted).JSON(accepted)
}

// GetStatus returns the current state of a generation job.
//
// @Summary      Get banner generation status
// @Description  Returns status (in_progress / completed / failed), image variants and failure info.
// @Description  Variants contain b64_json (base64 encoded PNG) when using gpt-image-1.
// @Tags         banner
// @Produce      json
// @Param        id   path      string  true  "Generation ID returned by POST /banner/generate"
// @Success      200  {object}  models.BannerGenerationResult
// @Failure      400  {object}  models.BannerErrorResponse
// @Failure      404  {object}  models.BannerErrorResponse
// @Router       /banner/generate/{id} [get]
func (h *BannerHandler) GetStatus(c *fiber.Ctx) error {
	generationID := c.Params("id")
	if generationID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.BannerErrorResponse{
			Error:     "Generation ID is required",
			Code:      "MISSING_ID",
			Timestamp: time.Now(),
		})
	}

	result, err := h.bannerService.GetStatus(generationID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.BannerErrorResponse{
			Error:     "Generation not found",
			Code:      "NOT_FOUND",
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	return c.JSON(result)
}

// GetVariantImage serves a generated variant as a raw PNG image.
// Open this URL directly in the browser to view the image.
//
// @Summary      View banner variant as image
// @Description  Decodes the b64_json of a variant and serves it as a PNG.
//
//	Open this URL directly in the browser to view the image.
//
// @Tags         banner
// @Produce      png
// @Param        id     path  string  true  "Generation ID"
// @Param        index  path  int     true  "Variant index (0-3)"
// @Success      200    {file}    binary
// @Failure      404    {object}  models.BannerErrorResponse
// @Router       /banner/generate/{id}/variant/{index}/image [get]
func (h *BannerHandler) GetVariantImage(c *fiber.Ctx) error {
	generationID := c.Params("id")
	indexStr := c.Params("index")

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.BannerErrorResponse{
			Error:     "Invalid variant index",
			Code:      "INVALID_INDEX",
			Message:   "index must be a number",
			Timestamp: time.Now(),
		})
	}

	result, err := h.bannerService.GetStatus(generationID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.BannerErrorResponse{
			Error:     "Generation not found",
			Code:      "NOT_FOUND",
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	if result.Status != models.GenerationStatusCompleted {
		return c.Status(fiber.StatusNotFound).JSON(models.BannerErrorResponse{
			Error:     "Generation not completed yet",
			Code:      "NOT_READY",
			Message:   fmt.Sprintf("current status: %s", result.Status),
			Timestamp: time.Now(),
		})
	}

	// Find the variant by index
	var variant *models.BannerVariant
	for i := range result.Variants {
		if result.Variants[i].Index == index {
			variant = &result.Variants[i]
			break
		}
	}

	if variant == nil {
		return c.Status(fiber.StatusNotFound).JSON(models.BannerErrorResponse{
			Error:     "Variant not found",
			Code:      "VARIANT_NOT_FOUND",
			Message:   fmt.Sprintf("variant index %d not found", index),
			Timestamp: time.Now(),
		})
	}

	// If we have a URL just redirect
	if variant.URL != "" {
		return c.Redirect(variant.URL)
	}

	// Decode b64 and serve as PNG
	if variant.B64JSON == "" {
		return c.Status(fiber.StatusNotFound).JSON(models.BannerErrorResponse{
			Error:     "No image data available",
			Code:      "NO_IMAGE_DATA",
			Timestamp: time.Now(),
		})
	}

	imageBytes, err := base64.StdEncoding.DecodeString(variant.B64JSON)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.BannerErrorResponse{
			Error:     "Failed to decode image",
			Code:      "DECODE_ERROR",
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	c.Set("Content-Type", "image/png")
	c.Set("Content-Disposition", fmt.Sprintf(`inline; filename="banner_%s_%d.png"`, generationID[:8], index))
	return c.Send(imageBytes)
}
