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
const defaultVariantCount = 4

// BannerServiceConfig holds tuneable parameters for BannerService.
type BannerServiceConfig struct {
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

	params := imagegen.PromptParams{
		Width:               req.Output.Width,
		Height:              req.Output.Height,
		Headline:            req.Inputs.Headline,
		SecondaryText:       req.Inputs.SecondaryText,
		CTAText:             req.Inputs.CTAText,
		VisualStyle:         req.Inputs.VisualStyle,
		CreativeDescription: req.Inputs.CreativeDescription,
	}

	// Build prompt synchronously so we fail fast on template errors.
	prompt, err := s.promptBuilder.Build(req.Language, params)
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
		Prompt: prompt,
		Width:  req.Output.Width,
		Height: req.Output.Height,
	}

	s.logger.WithFields(logrus.Fields{
		"generation_id":      generationID,
		"language":           req.Language,
		"provider":           s.provider.Name(),
		"variants":           s.variantCount,
		"has_headline":       req.Inputs.Headline != "",
		"has_secondary_text": req.Inputs.SecondaryText != "",
		"has_cta":            req.Inputs.CTAText != "",
		"has_creative_desc":  req.Inputs.CreativeDescription != "",
		"visual_style":       req.Inputs.VisualStyle,
	}).Info("Banner generation accepted")

	// Async fan-out — runs after HTTP response is sent.
	go s.generateVariants(context.Background(), generationID, genReq)

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
			// Stagger requests slightly to avoid simultaneous rate limit hits.
			time.Sleep(time.Duration(idx) * 500 * time.Millisecond)
			img, err := s.provider.Generate(ctx, req)
			results <- variantResult{index: idx, img: img, err: err}
		}(i)
	}

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

	// Sort variants by original index for deterministic ordering.
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

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// validateBannerRequest validates a BannerGenerationRequest against the
// following rules:
//
// Required fields:
//   - inputs.visual_style
//   - output.width  (must be positive)
//   - output.height (must be positive)
//
// Minimum content requirement — at least one of:
//   - inputs.headline
//   - inputs.cta_text
//   - inputs.creative_description
//
// Fully optional fields:
//   - inputs.secondary_text
//   - output.max_size_kb
//   - language
func validateBannerRequest(req *models.BannerGenerationRequest) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}

	// Required structural fields.
	if req.Inputs.VisualStyle == "" {
		return fmt.Errorf("inputs.visual_style is required")
	}
	if req.Output.Width <= 0 {
		return fmt.Errorf("output.width must be a positive integer")
	}
	if req.Output.Height <= 0 {
		return fmt.Errorf("output.height must be a positive integer")
	}

	// Minimum content requirement: at least one content-driving field.
	if req.Inputs.Headline == "" && req.Inputs.CTAText == "" && req.Inputs.CreativeDescription == "" {
		return fmt.Errorf(
			"at least one content field is required: inputs.headline, inputs.cta_text, or inputs.creative_description",
		)
	}

	return nil
}
