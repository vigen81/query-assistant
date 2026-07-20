package imagegen

import "context"

// GenerationRequest is the provider-agnostic image generation request.
//
// Callers supply a Preset rather than pixel dimensions; each provider maps
// the preset onto a canvas it actually supports (see Provider.ResolveSize).
type GenerationRequest struct {
	Prompt string
	Preset Preset
	// ResponseFormat is "url" or "b64_json". Defaults to "url".
	ResponseFormat string
}

// GeneratedImage is a single image returned by a provider.
type GeneratedImage struct {
	URL           string
	B64JSON       string
	RevisedPrompt string
	// GeneratedWidth/GeneratedHeight are the actual pixel dimensions the
	// provider produced for the requested preset. These may differ between
	// providers and models for the same preset.
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
	// ResolveSize maps a preset onto the canvas this provider will use.
	// The mapping is owned entirely by the provider, so swapping providers
	// or models never changes the client-facing contract. It returns an
	// error if the provider cannot serve the preset at all.
	ResolveSize(preset Preset) (Size, error)
	// Generate sends a single image generation request and returns the result.
	Generate(ctx context.Context, req GenerationRequest) (*GeneratedImage, error)
}
