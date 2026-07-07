package imagegen

import (
	"context"
	"fmt"
	"math"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

// gptImageBuckets are the only sizes gpt-image-1 accepts.
var gptImageBuckets = []struct {
	Width, Height int
}{
	{1024, 1024},
	{1024, 1536},
	{1536, 1024},
}

// closestGPTImageSize returns the gpt-image-1 bucket whose aspect ratio is
// closest to the requested width/height. gpt-image-1 has no arbitrary-size
// or aspect-ratio parameter, so any caller-supplied size must be snapped to
// one of its three fixed canvases before the API call.
func closestGPTImageSize(width, height int) (w, h int) {
	if width <= 0 || height <= 0 {
		return 1024, 1024
	}
	target := float64(width) / float64(height)

	bestW, bestH := gptImageBuckets[0].Width, gptImageBuckets[0].Height
	bestDiff := math.MaxFloat64
	for _, b := range gptImageBuckets {
		ratio := float64(b.Width) / float64(b.Height)
		if diff := math.Abs(ratio - target); diff < bestDiff {
			bestDiff = diff
			bestW, bestH = b.Width, b.Height
		}
	}
	return bestW, bestH
}

// OpenAIAdapter implements Provider using OpenAI gpt-image-1.
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

func (a *OpenAIAdapter) Name() string { return "openai-gpt-image-1" }

// Generate sends one gpt-image-1 image generation request.
// gpt-image-1 only supports n=1 per call; callers must fan-out for multiple variants.
// gpt-image-1 returns b64_json by default (no response_format parameter supported).
func (a *OpenAIAdapter) Generate(ctx context.Context, req GenerationRequest) (*GeneratedImage, error) {
	genW, genH := closestGPTImageSize(req.Width, req.Height)
	size := fmt.Sprintf("%dx%d", genW, genH)

	a.logger.WithFields(logrus.Fields{
		"requested_size":  fmt.Sprintf("%dx%d", req.Width, req.Height),
		"generation_size": size,
		"model":           "gpt-image-1",
	}).Debug("Sending gpt-image-1 generation request (snapped to nearest supported bucket)")

	resp, err := a.client.CreateImage(ctx, openai.ImageRequest{
		Model:  "gpt-image-1",
		Prompt: req.Prompt,
		N:      1,
		Size:   size,
		// response_format is NOT supported by gpt-image-1
		// gpt-image-1 returns b64_json by default
	})
	if err != nil {
		return nil, fmt.Errorf("gpt-image-1 generation failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("gpt-image-1 returned empty data")
	}

	a.logger.WithFields(logrus.Fields{
		"has_b64":            len(resp.Data[0].B64JSON) > 0,
		"has_url":            len(resp.Data[0].URL) > 0,
		"revised_prompt_len": len(resp.Data[0].RevisedPrompt),
	}).Debug("gpt-image-1 generation succeeded")

	return &GeneratedImage{
		URL:             resp.Data[0].URL,
		B64JSON:         resp.Data[0].B64JSON,
		RevisedPrompt:   resp.Data[0].RevisedPrompt,
		GeneratedWidth:  genW,
		GeneratedHeight: genH,
	}, nil
}
