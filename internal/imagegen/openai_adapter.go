package imagegen

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

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
	size := fmt.Sprintf("%dx%d", req.Width, req.Height)

	a.logger.WithFields(logrus.Fields{
		"size":  size,
		"model": "gpt-image-1",
	}).Debug("Sending gpt-image-1 generation request")

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
		URL:           resp.Data[0].URL,
		B64JSON:       resp.Data[0].B64JSON,
		RevisedPrompt: resp.Data[0].RevisedPrompt,
	}, nil
}
