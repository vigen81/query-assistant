package imagegen

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

// dall3Sizes lists the only sizes DALL-E 3 accepts.
// We map the requested dimensions to the closest one.
var dall3Sizes = []struct {
	w, h int
	tag  string
}{
	{1024, 1024, openai.CreateImageSize1024x1024},
	{1792, 1024, openai.CreateImageSize1792x1024},
	{1024, 1792, openai.CreateImageSize1024x1792},
}

// OpenAIAdapter implements Provider using OpenAI DALL-E 3.
type OpenAIAdapter struct {
	client *openai.Client
	logger *logrus.Logger
}

// NewOpenAIAdapter constructs the adapter from an existing sashabaranov client.
func NewOpenAIAdapter(client *openai.Client, logger *logrus.Logger) *OpenAIAdapter {
	return &OpenAIAdapter{client: client, logger: logger}
}

// NewOpenAIAdapterFromKey constructs the adapter directly from an API key.
func NewOpenAIAdapterFromKey(apiKey string, logger *logrus.Logger) *OpenAIAdapter {
	return &OpenAIAdapter{
		client: openai.NewClient(apiKey),
		logger: logger,
	}
}

func (a *OpenAIAdapter) Name() string { return "openai-dall-e-3" }

// Generate sends one DALL-E 3 image generation request.
// DALL-E 3 only supports n=1 per call; callers must fan-out for multiple variants.
func (a *OpenAIAdapter) Generate(ctx context.Context, req GenerationRequest) (*GeneratedImage, error) {
	size := a.nearestSize(req.Width, req.Height)

	respFmt := openai.CreateImageResponseFormatURL
	if req.ResponseFormat == "b64_json" {
		respFmt = openai.CreateImageResponseFormatB64JSON
	}

	a.logger.WithFields(logrus.Fields{
		"size":   size,
		"format": respFmt,
	}).Debug("Sending DALL-E 3 generation request")

	resp, err := a.client.CreateImage(ctx, openai.ImageRequest{
		Model:          openai.CreateImageModelDallE3,
		Prompt:         req.Prompt,
		N:              1, // DALL-E 3 hard limit
		Size:           size,
		Quality:        openai.CreateImageQualityStandard,
		ResponseFormat: respFmt,
	})
	if err != nil {
		return nil, fmt.Errorf("dall-e-3 generation failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("dall-e-3 returned empty data")
	}

	img := &GeneratedImage{
		URL:           resp.Data[0].URL,
		B64JSON:       resp.Data[0].B64JSON,
		RevisedPrompt: resp.Data[0].RevisedPrompt,
	}

	a.logger.WithField("revised_prompt_len", len(img.RevisedPrompt)).
		Debug("DALL-E 3 generation succeeded")

	return img, nil
}

// nearestSize returns the DALL-E 3 size tag whose dimensions are closest
// (by Manhattan distance) to the requested dimensions.
func (a *OpenAIAdapter) nearestSize(w, h int) string {
	best := dall3Sizes[0]
	bestDist := abs(w-best.w) + abs(h-best.h)

	for _, s := range dall3Sizes[1:] {
		d := abs(w-s.w) + abs(h-s.h)
		if d < bestDist {
			best = s
			bestDist = d
		}
	}
	return best.tag
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
