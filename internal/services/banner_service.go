package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/imagegen"
	"gitlab.smartbet.am/golang/query-assistant/internal/models"
)

// defaultVariantCount is the number of image variants generated per request.
// This is overridable via BannerServiceConfig.
const defaultVariantCount = 4

// BannerServiceConfig holds tuneable parameters for BannerService.
type BannerServiceConfig struct {
	// VariantCount is how many image variants to generate per request.
	// Defaults to 4.
	VariantCount int
}

// BannerService orchestrates async banner image generation.
type BannerService struct {
	provider      imagegen.Provider
	promptBuilder *imagegen.PromptBuilder
	store         *imagegen.GenerationStore
	variantCount  int
	logger        *logrus.Logger
}

// NewBannerService constructs the service.
// cfg may be nil — defaults will be applied.
func NewBannerService(
	provider imagegen.Provider,
	promptBuilder *imagegen.PromptBuilder,
	store *imagegen.GenerationStore,
	cfg *BannerServiceConfig,
	logger *logrus.Logger,
) *BannerService {
	vc := defaultVariantCount
	if cfg != nil && cfg.VariantCount > 0 {
		vc = cfg.VariantCount
	}
	return &BannerService{
		provider:      provider,
		promptBuilder: promptBuilder,
		store:         store,
		variantCount:  vc,
		logger:        logger,
	}
}

// Generate accepts a banner generation request, stores an in-progress entry,
// kicks off async generation, and returns the generation ID immediately.
func (s *BannerService) Generate(ctx context.Context, req *models.BannerGenerationRequest) (*models.BannerGenerationAccepted, error) {
	if err := validateBannerRequest(req); err != nil {
		return nil, err
	}

	if req.Language == "" {
		req.Language = "en"
	}
	if req.Output.Format == "" {
		req.Output.Format = "url"
	}

	// Build prompt synchronously so we can return an error before storing anything.
	prompt, err := s.promptBuilder.Build(req.Language, imagegen.PromptParams{
		Width:               req.Output.Width,
		Height:              req.Output.Height,
		Headline:            req.Inputs.Headline,
		SecondaryText:       req.Inputs.SecondaryText,
		CTAText:             req.Inputs.CTAText,
		VisualStyle:         req.Inputs.VisualStyle,
		CreativeDescription: req.Inputs.CreativeDescription,
	})
	if err != nil {
		return nil, fmt.Errorf("prompt build failed: %w", err)
	}

	generationID := uuid.New().String()
	now := time.Now()

	s.store.Create(&models.BannerGenerationResult{
		GenerationID: generationID,
		Status:       models.GenerationStatusInProgress,
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	genReq := imagegen.GenerationRequest{
		Prompt:         prompt,
		Width:          req.Output.Width,
		Height:         req.Output.Height,
		ResponseFormat: req.Output.Format,
	}

	// Async generation — runs after the HTTP response is sent.
	go s.generateVariants(context.Background(), generationID, genReq)

	s.logger.WithFields(logrus.Fields{
		"generation_id": generationID,
		"language":      req.Language,
		"provider":      s.provider.Name(),
		"variants":      s.variantCount,
	}).Info("Banner generation accepted")

	return &models.BannerGenerationAccepted{
		GenerationID: generationID,
		Status:       models.GenerationStatusInProgress,
		CreatedAt:    now,
	}, nil
}

// GetStatus returns the current state of a generation job.
func (s *BannerService) GetStatus(generationID string) (*models.BannerGenerationResult, error) {
	return s.store.Get(generationID)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

type variantResult struct {
	index int
	img   *imagegen.GeneratedImage
	err   error
}

// generateVariants fans-out N concurrent provider calls and writes the
// aggregated outcome back to the store.
func (s *BannerService) generateVariants(ctx context.Context, generationID string, req imagegen.GenerationRequest) {
	results := make(chan variantResult, s.variantCount)
	var wg sync.WaitGroup

	for i := 0; i < s.variantCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			img, err := s.provider.Generate(ctx, req)
			results <- variantResult{index: idx, img: img, err: err}
		}(i)
	}

	// Close results channel once all goroutines finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	var (
		variants []models.BannerVariant
		failures []string
	)

	for r := range results {
		if r.err != nil {
			s.logger.WithFields(logrus.Fields{
				"generation_id": generationID,
				"variant_index": r.index,
				"error":         r.err.Error(),
			}).Error("Variant generation failed")
			failures = append(failures, fmt.Sprintf("variant %d: %s", r.index, r.err.Error()))
			continue
		}
		variants = append(variants, models.BannerVariant{
			Index:         r.index,
			URL:           r.img.URL,
			B64JSON:       r.img.B64JSON,
			RevisedPrompt: r.img.RevisedPrompt,
		})
	}

	if len(variants) == 0 {
		reason := strings.Join(failures, "; ")
		s.logger.WithFields(logrus.Fields{
			"generation_id": generationID,
			"reason":        reason,
		}).Error("All variants failed — marking generation as failed")
		s.store.MarkFailed(generationID, reason)
		return
	}

	// Sort variants by their original index for deterministic ordering.
	sort.Slice(variants, func(i, j int) bool {
		return variants[i].Index < variants[j].Index
	})

	if len(failures) > 0 {
		s.logger.WithFields(logrus.Fields{
			"generation_id": generationID,
			"succeeded":     len(variants),
			"failed":        len(failures),
		}).Warn("Partial variant success — completing with available variants")
	}

	s.store.MarkCompleted(generationID, variants)

	s.logger.WithFields(logrus.Fields{
		"generation_id": generationID,
		"variants":      len(variants),
	}).Info("Banner generation completed")
}

// validateBannerRequest performs lightweight structural validation.
func validateBannerRequest(req *models.BannerGenerationRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if req.Inputs.Headline == "" {
		return fmt.Errorf("inputs.headline is required")
	}
	if req.Inputs.SecondaryText == "" {
		return fmt.Errorf("inputs.secondary_text is required")
	}
	if req.Inputs.CTAText == "" {
		return fmt.Errorf("inputs.cta_text is required")
	}
	if req.Inputs.VisualStyle == "" {
		return fmt.Errorf("inputs.visual_style is required")
	}
	if req.Output.Width <= 0 {
		return fmt.Errorf("output.width must be positive")
	}
	if req.Output.Height <= 0 {
		return fmt.Errorf("output.height must be positive")
	}
	return nil
}
