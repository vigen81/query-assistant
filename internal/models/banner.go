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
	SecondaryText       string `json:"secondary_text"        example:"Up to $500 matched. T&Cs apply."`
	CTAText             string `json:"cta_text"              example:"Claim Now"`
	VisualStyle         string `json:"visual_style"          example:"dark_luxury"`
	CreativeDescription string `json:"creative_description"  example:"Gold coins and dramatic lighting"`
}

// BannerOutputParams defines the desired output characteristics.
type BannerOutputParams struct {
	Width     int    `json:"width"          example:"1792"`
	Height    int    `json:"height"         example:"1024"`
	Format    string `json:"format"         example:"url"`
	MaxSizeKB int    `json:"max_size_kb"    example:"2048"`
}

// BannerGenerationRequest is the payload sent to POST /banner/generate.
type BannerGenerationRequest struct {
	Language string             `json:"language" validate:"required"` // e.g. "en"
	Inputs   BannerInputs       `json:"inputs"   validate:"required"`
	Output   BannerOutputParams `json:"output"   validate:"required"`
}

// BannerGenerationAccepted is returned immediately (HTTP 202) after the request is accepted.
type BannerGenerationAccepted struct {
	GenerationID string    `json:"generation_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Status       string    `json:"status"        example:"in_progress"`
	CreatedAt    time.Time `json:"created_at"    example:"2024-01-01T00:00:00Z"`
}

// BannerVariant represents a single generated image variant.
type BannerVariant struct {
	Index         int    `json:"index"          example:"0"`
	URL           string `json:"url"            example:"https://oaidalleapiprodscus.blob.core.windows.net/..."`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt" example:"A high-quality promotional banner..."`
}

// BannerGenerationResult is returned by GET /banner/generate/:id.
type BannerGenerationResult struct {
	GenerationID  string          `json:"generation_id"            example:"550e8400-e29b-41d4-a716-446655440000"`
	Status        string          `json:"status"                   example:"completed"`
	Variants      []BannerVariant `json:"variants,omitempty"`
	FailureReason string          `json:"failure_reason,omitempty" example:"dall-e-3 generation failed: rate limit"`
	CreatedAt     time.Time       `json:"created_at"               example:"2024-01-01T00:00:00Z"`
	UpdatedAt     time.Time       `json:"updated_at"               example:"2024-01-01T00:00:05Z"`
}

// BannerErrorResponse is the error response for banner endpoints.
type BannerErrorResponse struct {
	Error     string    `json:"error"             example:"Banner generation request failed"`
	Code      string    `json:"code"              example:"VALIDATION_ERROR"`
	Message   string    `json:"message,omitempty" example:"inputs.headline is required"`
	Timestamp time.Time `json:"timestamp"         example:"2024-01-01T00:00:00Z"`
}
