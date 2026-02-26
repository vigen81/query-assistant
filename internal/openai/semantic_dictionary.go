package openai

import (
	"os"

	"gitlab.smartbet.am/golang/query-assistant/internal/logger"
)

// =============================================================================
// SEMANTIC DICTIONARY — ENVIRONMENT ROUTER
// =============================================================================
// Routes to the correct semantic dictionary version based on POD_ENV:
//   - dev / local  → v1.2.2 (semantic_dictionary_dev.go)
//   - staging      → v1.0.0 (semantic_dictionary_prod.go)  — same as prod for now
//   - prod         → v1.0.0 (semantic_dictionary_prod.go)
//
// Files:
//   semantic_dictionary.go      — this file (router)
//   semantic_dictionary_dev.go  — dev constants (v1.2.2)
//   semantic_dictionary_prod.go — prod constants (v1.0.0)
// =============================================================================

// GetSemanticVersion returns the dictionary version for the current environment.
func GetSemanticVersion() string {
	if isProdEnv() {
		return ProdSemanticVersion
	}
	return DevSemanticVersion
}

// GetSystemPrompt returns the system prompt for the current environment.
func GetSystemPrompt() string {
	if isProdEnv() {
		return ProdSystemPrompt
	}
	return DevSystemPrompt
}

// GetSemanticDictionary returns the semantic dictionary for the current environment.
func GetSemanticDictionary() string {
	if isProdEnv() {
		return ProdSemanticDictionary
	}
	return DevSemanticDictionary
}

// GetDDLSchema returns the DDL schema for the current environment.
func GetDDLSchema() string {
	if isProdEnv() {
		return "" //ProdDDLSchema
	}
	return DevDDLSchema
}

// isProdEnv returns true if the current environment is production or staging.
func isProdEnv() bool {
	env := os.Getenv("POD_ENV")
	logger.Log.Info("POD_ENV: ", env)
	return env == "prod" || env == "staging"
}
