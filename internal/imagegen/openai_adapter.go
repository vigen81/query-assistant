package imagegen

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

// gptImageModel is the model this adapter targets.
const gptImageModel = "gpt-image-1"

// gptImagePresetSizes maps each supported preset onto one of the three fixed
// canvases gpt-image-1 accepts. gpt-image-1 has no arbitrary-size or
// aspect-ratio parameter, so this table is the complete set of sizes this
// adapter can ever produce.
//
// A different provider or model may map the same presets onto different
// pixel sizes; that is intentional and invisible to clients.
var gptImagePresetSizes = map[Preset]Size{
	PresetSquare:    {Width: 1024, Height: 1024},
	PresetPortrait:  {Width: 1024, Height: 1536},
	PresetLandscape: {Width: 1536, Height: 1024},
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

func (a *OpenAIAdapter) Name() string { return "openai-" + gptImageModel }

// ResolveSize maps a preset onto the gpt-image-1 canvas used for it.
func (a *OpenAIAdapter) ResolveSize(preset Preset) (Size, error) {
	size, ok := gptImagePresetSizes[preset]
	if !ok {
		return Size{}, fmt.Errorf("%s does not support image preset %q", gptImageModel, preset)
	}
	return size, nil
}

// Generate sends one gpt-image-1 image generation request.
// gpt-image-1 only supports n=1 per call; callers must fan-out for multiple variants.
// gpt-image-1 returns b64_json by default (no response_format parameter supported).
func (a *OpenAIAdapter) Generate(ctx context.Context, req GenerationRequest) (*GeneratedImage, error) {
	size, err := a.ResolveSize(req.Preset)
	if err != nil {
		return nil, err
	}

	a.logger.WithFields(logrus.Fields{
		"image_preset":  string(req.Preset),
		"resolved_size": size.String(),
		"model":         gptImageModel,
	}).Debug("Sending gpt-image-1 generation request")

	resp, err := a.client.CreateImage(ctx, openai.ImageRequest{
		Model:  gptImageModel,
		Prompt: req.Prompt,
		N:      1,
		Size:   size.String(),
		// response_format is NOT supported by gpt-image-1
		// gpt-image-1 returns b64_json by default
	})
	if err != nil {
		return nil, fmt.Errorf("%s generation failed: %w", gptImageModel, err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("%s returned empty data", gptImageModel)
	}

	a.logger.WithFields(logrus.Fields{
		"has_b64":            len(resp.Data[0].B64JSON) > 0,
		"has_url":            len(resp.Data[0].URL) > 0,
		"revised_prompt_len": len(resp.Data[0].RevisedPrompt),
		"image_preset":       string(req.Preset),
		"resolved_size":      size.String(),
	}).Debug("gpt-image-1 generation succeeded")

	return &GeneratedImage{
		URL:             resp.Data[0].URL,
		B64JSON:         resp.Data[0].B64JSON,
		RevisedPrompt:   resp.Data[0].RevisedPrompt,
		GeneratedWidth:  size.Width,
		GeneratedHeight: size.Height,
	}, nil
}
