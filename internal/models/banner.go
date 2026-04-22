package models

import "time"

const (
	GenerationStatusInProgress = "in_progress"
	GenerationStatusCompleted  = "completed"
	GenerationStatusFailed     = "failed"
)

// BannerInputs holds the user-supplied creative content for the banner.
//
// Required fields:
//   - visual_style
//
// Optional fields (but at least one of headline, cta_text, or creative_description must be provided):
//   - headline
//   - secondary_text
//   - cta_text
//   - creative_description
type BannerInputs struct {
	// Headline is the main promotional text. Optional, but recommended for most banners.
	Headline string `json:"headline,omitempty" example:"Get 100% Bonus on First Deposit"`
	// SecondaryText is supporting copy displayed near the headline. Fully optional.
	SecondaryText string `json:"secondary_text,omitempty" example:"Up to $500 matched. Terms apply."`
	// CTAText is the call-to-action button label. Optional but strongly recommended.
	CTAText string `json:"cta_text,omitempty" example:"Claim Now"`
	// VisualStyle is the visual preset applied to the banner. Required.
	VisualStyle string `json:"visual_style" example:"dark_luxury"`
	// CreativeDescription provides additional artistic direction. Optional.
	CreativeDescription string `json:"creative_description,omitempty" example:"Abstract geometric shapes, gold and dark background, premium feel"`
}

// BannerOutputParams defines the desired output characteristics.
// Width and Height are required.
type BannerOutputParams struct {
	// Width is the output image width in pixels. Required.
	Width int `json:"width" example:"1024"`
	// Height is the output image height in pixels. Required.
	Height int `json:"height" example:"1024"`
	// MaxSizeKB is the maximum acceptable file size in kilobytes. Optional.
	MaxSizeKB int `json:"max_size_kb,omitempty" example:"2048"`
}

// BannerGenerationRequest is the payload sent to POST /banner/generate.
//
// Minimum valid request:
//   - inputs.visual_style  (required)
//   - output.width         (required)
//   - output.height        (required)
//   - At least one of: inputs.headline, inputs.cta_text, inputs.creative_description
type BannerGenerationRequest struct {
	// Language controls the prompt template locale. Defaults to "en" when omitted.
	Language string `json:"language,omitempty" example:"en"`
	// Inputs holds the creative content fields.
	Inputs BannerInputs `json:"inputs"`
	// Output defines the desired image dimensions and size constraints.
	Output BannerOutputParams `json:"output"`
}

// BannerGenerationAccepted is returned immediately (HTTP 202) after the request is accepted.
type BannerGenerationAccepted struct {
	GenerationID string    `json:"generation_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Status       string    `json:"status"        example:"in_progress"`
	CreatedAt    time.Time `json:"created_at"    example:"2024-01-01T00:00:00Z"`
}

// BannerVariantMetadata holds inspected file properties.
type BannerVariantMetadata struct {
	Width       int    `json:"width"        example:"1024"`
	Height      int    `json:"height"       example:"1024"`
	FileSizeKB  int    `json:"file_size_kb" example:"856"`
	ContentType string `json:"content_type" example:"image/png"`
}

// BannerVariant represents a single generated image variant.
// gpt-image-1 returns b64_json by default; URL may be empty.
type BannerVariant struct {
	Index         int                    `json:"index"                    example:"0"`
	URL           string                 `json:"url,omitempty"            example:"https://oaidalleapiprodscus.blob.core.windows.net/private/..."`
	B64JSON       string                 `json:"b64_json,omitempty"`
	RevisedPrompt string                 `json:"revised_prompt,omitempty" example:"A high-quality promotional banner with dark luxury style..."`
	Metadata      *BannerVariantMetadata `json:"metadata,omitempty"`
}

// BannerGenerationResult is returned by GET /banner/generate/:id.
type BannerGenerationResult struct {
	GenerationID  string          `json:"generation_id"            example:"550e8400-e29b-41d4-a716-446655440000"`
	Status        string          `json:"status"                   example:"completed"`
	Variants      []BannerVariant `json:"variants,omitempty"`
	FailureReason string          `json:"failure_reason,omitempty" example:"gpt-image-1 generation failed: rate limit exceeded"`
	CreatedAt     time.Time       `json:"created_at"               example:"2024-01-01T00:00:00Z"`
	UpdatedAt     time.Time       `json:"updated_at"               example:"2024-01-01T00:00:15Z"`
}

// BannerErrorResponse is the error response specific to banner endpoints.
type BannerErrorResponse struct {
	Error     string    `json:"error"             example:"Banner generation request failed"`
	Code      string    `json:"code"              example:"VALIDATION_ERROR"`
	Message   string    `json:"message,omitempty" example:"at least one content field is required: headline, cta_text, or creative_description"`
	Timestamp time.Time `json:"timestamp"         example:"2024-01-01T00:00:00Z"`
}

// BannerValidationError codes used in BannerErrorResponse.Code.
const (
	BannerErrCodeInvalidRequest  = "INVALID_REQUEST"
	BannerErrCodeValidation      = "VALIDATION_ERROR"
	BannerErrCodeMissingContent  = "MISSING_CONTENT"
	BannerErrCodeNotFound        = "NOT_FOUND"
	BannerErrCodeNotReady        = "NOT_READY"
	BannerErrCodeInvalidIndex    = "INVALID_INDEX"
	BannerErrCodeVariantNotFound = "VARIANT_NOT_FOUND"
	BannerErrCodeNoImageData     = "NO_IMAGE_DATA"
	BannerErrCodeDecodeError     = "DECODE_ERROR"
)
