package openai

import "os"

// semantic_dictionary.go
// Public API for semantic dictionary and system prompt.
// Routes to prod or dev constants based on POD_ENV.
//
// Prod  → system prompt v1.1.0  |  semantic dictionary v1.1.4
// Dev   → system prompt v1.2.x  |  semantic dictionary v1.2.2

const (
	prodSemanticVersion = "v2.5"
	devSemanticVersion  = "v2.5"
)

func isProdEnv() bool {
	return os.Getenv("POD_ENV") == "prod"
}

// GetSystemPrompt returns the system prompt for the current environment.
func GetSystemPrompt() string {
	if isProdEnv() {
		return prodSystemPrompt
	}
	return devSystemPrompt
}

// GetSemanticDictionary returns the semantic dictionary for the current environment.
func GetSemanticDictionary() string {
	if isProdEnv() {
		return prodSemanticDictionary
	}
	return devSemanticDictionary
}

// GetDDLSchema returns the DDL schema for the current environment.
// DDL is disabled in prod (returns empty string).
func GetDDLSchema() string {
	if isProdEnv() {
		return ""
	}
	return ""
}

// GetSemanticVersion returns the semantic dictionary version for the current environment.
func GetSemanticVersion() string {
	if isProdEnv() {
		return prodSemanticVersion
	}
	return devSemanticVersion
}
