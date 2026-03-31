package models

import "time"

const (
	GenerationStatusInProgress = "in_progress"
	GenerationStatusCompleted  = "completed"
	GenerationStatusFailed     = "failed"
)

// BannerInputs holds the user-supplied creative content for the banner.
type BannerInputs struct {
	Headline            string `json:"headline"              example:"Get 100% Bonus on First Deposit"`
	SecondaryText       string `json:"secondary_text"        example:"Up to $500 matched. Terms apply."`
	CTAText             string `json:"cta_text"              example:"Claim Now"`
	VisualStyle         string `json:"visual_style"          example:"dark_luxury"`
	CreativeDescription string `json:"creative_description"  example:"Abstract geometric shapes, gold and dark background, premium feel"`
}

// BannerOutputParams defines the desired output characteristics.
type BannerOutputParams struct {
	Width     int `json:"width"          example:"1024"`
	Height    int `json:"height"         example:"1024"`
	MaxSizeKB int `json:"max_size_kb"    example:"2048"`
}

// BannerGenerationRequest is the payload sent to POST /banner/generate.
type BannerGenerationRequest struct {
	Language string             `json:"language" example:"en"`
	Inputs   BannerInputs       `json:"inputs"`
	Output   BannerOutputParams `json:"output"`
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
	Message   string    `json:"message,omitempty" example:"inputs.headline is required"`
	Timestamp time.Time `json:"timestamp"         example:"2024-01-01T00:00:00Z"`
}
