package handlers

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
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
// @Description  Starts an async AI banner generation job and returns a generation ID immediately (HTTP 202).
// @Description  Poll GET /banner/generate/{id} until status is "completed" or "failed".
// @Description
// @Description  ## Required fields
// @Description  - `inputs.visual_style` — visual preset (e.g. "dark_luxury", "neon_sport")
// @Description  - At least one of: `inputs.headline`, `inputs.cta_text`, `inputs.creative_description`
// @Description
// @Description  ## Image preset
// @Description  `output.image_preset` selects the image orientation. Supported values:
// @Description  - `square` (default when omitted)
// @Description  - `portrait`
// @Description  - `landscape`
// @Description
// @Description  The service maps the preset onto a canvas supported by the configured
// @Description  AI provider and model. Clients must not send `output.width` or
// @Description  `output.height` — such requests are rejected with a validation error.
// @Description
// @Description  ## Optional fields
// @Description  - `inputs.secondary_text` — supporting copy (always optional)
// @Description  - `inputs.headline`        — main promotional text
// @Description  - `inputs.cta_text`        — call-to-action button label
// @Description  - `inputs.creative_description` — additional artistic direction
// @Description  - `output.image_preset`    — orientation, defaults to "square"
// @Description  - `output.max_size_kb`     — maximum file size hint
// @Description  - `language`               — prompt locale, defaults to "en"
// @Description
// @Description  ## Minimum valid requests (examples)
// @Description  ```json
// @Description  { "inputs": { "visual_style": "dark_luxury", "headline": "Claim Your Bonus" },
// @Description    "output": { "image_preset": "landscape" } }
// @Description  ```
// @Description  ```json
// @Description  { "inputs": { "visual_style": "neon_sport", "cta_text": "Play Now" } }
// @Description  ```
// @Description  ```json
// @Description  { "inputs": { "visual_style": "gold_premium",
// @Description                "creative_description": "Abstract geometric shapes, dark background" },
// @Description    "output": { "image_preset": "portrait" } }
// @Description  ```
// @Description
// @Description  ## Image data
// @Description  gpt-image-1 returns `b64_json` by default. The `url` field may be empty.
// @Description  Use GET /banner/generate/{id}/variant/{index}/image to render the PNG directly.
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
			Code:      models.BannerErrCodeInvalidRequest,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	accepted, err := h.bannerService.Generate(c.Context(), &req)
	if err != nil {
		h.logger.WithError(err).Warn("Banner generation request rejected")

		code := models.BannerErrCodeValidation
		// Distinguish missing-content errors for clearer client messaging.
		if isContentError(err.Error()) {
			code = models.BannerErrCodeMissingContent
		}

		return c.Status(fiber.StatusBadRequest).JSON(models.BannerErrorResponse{
			Error:     "Banner generation request failed",
			Code:      code,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	return c.Status(fiber.StatusAccepted).JSON(accepted)
}

// GetStatus returns the current state of a generation job.
//
// @Summary      Poll banner generation status
// @Description  Returns the current status of a generation job.
// @Description
// @Description  Possible status values:
// @Description  - `in_progress` — generation is running; poll again in a few seconds
// @Description  - `completed`   — all (or partial) variants are ready
// @Description  - `failed`      — all variants failed; see failure_reason
// @Description
// @Description  When using gpt-image-1, variants contain `b64_json` (base64 PNG).
// @Description  The `url` field may be empty. Use the /variant/{index}/image endpoint
// @Description  to view variants directly in a browser.
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
			Code:      models.BannerErrCodeInvalidRequest,
			Timestamp: time.Now(),
		})
	}

	result, err := h.bannerService.GetStatus(generationID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.BannerErrorResponse{
			Error:     "Generation not found",
			Code:      models.BannerErrCodeNotFound,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	return c.JSON(result)
}

// GetVariantImage serves a generated variant as a raw PNG image.
//
// @Summary      View banner variant as PNG
// @Description  Decodes the b64_json of a completed variant and serves it as image/png.
// @Description  Open this URL directly in a browser to preview the image.
// @Description  If the variant has a URL (non-gpt-image-1 providers), redirects to that URL instead.
// @Tags         banner
// @Produce      png
// @Param        id     path  string  true  "Generation ID"
// @Param        index  path  int     true  "Variant index (0-based, up to variant_count-1)"
// @Success      200    {file}    binary
// @Failure      400    {object}  models.BannerErrorResponse
// @Failure      404    {object}  models.BannerErrorResponse
// @Failure      500    {object}  models.BannerErrorResponse
// @Router       /banner/generate/{id}/variant/{index}/image [get]
func (h *BannerHandler) GetVariantImage(c *fiber.Ctx) error {
	generationID := c.Params("id")
	indexStr := c.Params("index")

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.BannerErrorResponse{
			Error:     "Invalid variant index",
			Code:      models.BannerErrCodeInvalidIndex,
			Message:   "index must be a non-negative integer",
			Timestamp: time.Now(),
		})
	}

	result, err := h.bannerService.GetStatus(generationID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.BannerErrorResponse{
			Error:     "Generation not found",
			Code:      models.BannerErrCodeNotFound,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	if result.Status != models.GenerationStatusCompleted {
		return c.Status(fiber.StatusNotFound).JSON(models.BannerErrorResponse{
			Error:     "Generation not completed yet",
			Code:      models.BannerErrCodeNotReady,
			Message:   fmt.Sprintf("current status: %s", result.Status),
			Timestamp: time.Now(),
		})
	}

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
			Code:      models.BannerErrCodeVariantNotFound,
			Message:   fmt.Sprintf("variant index %d not found", index),
			Timestamp: time.Now(),
		})
	}

	// If we have a direct URL, redirect.
	if variant.URL != "" {
		return c.Redirect(variant.URL)
	}

	if variant.B64JSON == "" {
		return c.Status(fiber.StatusNotFound).JSON(models.BannerErrorResponse{
			Error:     "No image data available for this variant",
			Code:      models.BannerErrCodeNoImageData,
			Timestamp: time.Now(),
		})
	}

	imageBytes, err := base64.StdEncoding.DecodeString(variant.B64JSON)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.BannerErrorResponse{
			Error:     "Failed to decode image data",
			Code:      models.BannerErrCodeDecodeError,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
	}

	c.Set("Content-Type", "image/png")
	c.Set("Content-Disposition", fmt.Sprintf(`inline; filename="banner_%s_%d.png"`, generationID[:8], index))
	return c.Send(imageBytes)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isContentError returns true when the error message indicates a missing-content
// violation, allowing the handler to use a more specific error code.
func isContentError(msg string) bool {
	return strings.Contains(msg, "at least one") &&
		(strings.Contains(msg, "headline") ||
			strings.Contains(msg, "cta_text") ||
			strings.Contains(msg, "creative_description"))
}
