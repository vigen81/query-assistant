package imagegen

import "context"

// GenerationRequest is the provider-agnostic image generation request.
type GenerationRequest struct {
	Prompt string
	Width  int
	Height int
	// ResponseFormat is "url" or "b64_json". Defaults to "url".
	ResponseFormat string
}

// GeneratedImage is a single image returned by a provider.
type GeneratedImage struct {
	URL           string
	B64JSON       string
	RevisedPrompt string
	// GeneratedWidth/GeneratedHeight are the actual pixel dimensions the
	// provider produced. For gpt-image-1 this is one of its fixed buckets
	// and may differ from the caller's requested Width/Height.
	GeneratedWidth  int
	GeneratedHeight int
}

// Provider is the interface every image-generation backend must satisfy.
// Each call to Generate produces exactly one image so that callers can
// fan-out N concurrent calls for N variants (required by DALL-E 3 which
// enforces n=1 per request).
type Provider interface {
	// Name returns a human-readable identifier for the provider.
	Name() string
	// Generate sends a single image generation request and returns the result.
	Generate(ctx context.Context, req GenerationRequest) (*GeneratedImage, error)
}
