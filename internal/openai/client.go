package openai

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/config"
	"gitlab.smartbet.am/golang/query-assistant/internal/models"
)

type Client struct {
	client *openai.Client
	config *config.Config
	logger *logrus.Logger
}

func NewClient(cfg *config.Config, logger *logrus.Logger) *Client {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = cfg.OpenAI.APIKey
	}

	if apiKey == "" {
		logger.Error("OpenAI API key is empty!")
	} else {
		maskedKey := apiKey[:8] + "..." + apiKey[len(apiKey)-4:]
		logger.WithField("api_key", maskedKey).Info("OpenAI client initialized with API key")
	}

	client := openai.NewClient(apiKey)

	return &Client{
		client: client,
		config: cfg,
		logger: logger,
	}
}

// GenerateQuery generates a ClickHouse query from a natural language prompt using the semantic dictionary
func (c *Client) GenerateQuery(ctx context.Context, prompt string, schemaInfo *models.SchemaInfo, siteID int64) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.config.GetOpenAITimeout())
	defer cancel()

	// Build the system message with semantic dictionary
	systemMessage := c.buildSemanticDictionarySystemMessage(siteID)

	// Build the user message
	userMessage := fmt.Sprintf(`Generate a ClickHouse SQL query for the following request: "%s"

SITE CONTEXT: All queries MUST filter by site_id = %d (numeric, no quotes)

Remember:
- Use ONLY tables, columns, metrics, and joins defined in the Semantic Dictionary
- Apply default filters (is_test=0, is_rollback=0, status IN (1,2) for payments)
- Include LIMIT 1000 unless specified otherwise
- GGR = Bets - Wins (ALWAYS)
- Return ONLY the raw SQL query - no markdown, no explanations`, prompt, siteID)

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = c.config.OpenAI.Model
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemMessage,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: userMessage,
		},
	}

	c.logger.WithFields(logrus.Fields{
		"prompt":  prompt,
		"site_id": siteID,
		"model":   model,
	}).Debug("Requesting query generation from OpenAI with semantic dictionary")

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:       model,
			Messages:    messages,
			MaxTokens:   c.config.OpenAI.MaxTokens,
			Temperature: c.config.OpenAI.Temperature,
		},
	)

	if err != nil {
		c.logger.WithFields(logrus.Fields{
			"error":   err.Error(),
			"model":   model,
			"site_id": siteID,
		}).Error("OpenAI API request failed")
		return "", fmt.Errorf("failed to generate query: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	generatedQuery := strings.TrimSpace(resp.Choices[0].Message.Content)

	// Check if this is a predefined response (clarification or off-topic)
	if c.isPredefinedResponse(generatedQuery) {
		c.logger.WithFields(logrus.Fields{
			"response": generatedQuery,
			"site_id":  siteID,
		}).Info("Received predefined response from AI")
		return generatedQuery, nil
	}

	// Validate site_id presence for SQL queries
	if !c.validateSiteIDPresent(generatedQuery, siteID) {
		c.logger.WithFields(logrus.Fields{
			"generated_query": generatedQuery,
			"site_id":         siteID,
		}).Warn("Generated query missing site_id filter, attempting to add")
		generatedQuery = c.ensureSiteIDFilter(generatedQuery, siteID)
	}

	c.logger.WithFields(logrus.Fields{
		"generated_query": generatedQuery,
		"site_id":         siteID,
		"tokens_used":     resp.Usage.TotalTokens,
	}).Info("Query generated successfully")

	return generatedQuery, nil
}

// buildSemanticDictionarySystemMessage creates the system message using the SemanticDictionary constant
func (c *Client) buildSemanticDictionarySystemMessage(siteID int64) string {
	// Use the SemanticDictionary constant from semantic_dictionary.go and inject the site_id
	return fmt.Sprintf(`%s

---

## CURRENT QUERY CONTEXT

**Site ID**: %d
**CRITICAL**: All queries MUST filter by site_id = %d (numeric, no quotes)

Apply this filter to:
- Every table that has a site_id column
- Both sides of JOINs where both tables have site_id
- Subqueries and CTEs
`, SemanticDictionary, siteID, siteID)
}

// isPredefinedResponse checks if the response is a predefined message (not SQL)
func (c *Client) isPredefinedResponse(response string) bool {
	predefinedPrefixes := []string{
		"I can only generate reports",
		"Clarification required:",
	}

	for _, prefix := range predefinedPrefixes {
		if strings.HasPrefix(response, prefix) {
			return true
		}
	}
	return false
}

// validateSiteIDPresent checks if the query contains proper numeric site_id filtering
func (c *Client) validateSiteIDPresent(query string, siteID int64) bool {
	lowerQuery := strings.ToLower(query)

	if !strings.Contains(lowerQuery, "site_id") {
		return false
	}

	expectedPatterns := []string{
		fmt.Sprintf("site_id = %d", siteID),
		fmt.Sprintf("site_id=%d", siteID),
	}

	for _, pattern := range expectedPatterns {
		if strings.Contains(lowerQuery, strings.ToLower(pattern)) {
			return true
		}
	}

	// Check for quoted values (incorrect)
	quotedPatterns := []string{
		fmt.Sprintf("site_id = '%d'", siteID),
		fmt.Sprintf("site_id='%d'", siteID),
	}

	for _, pattern := range quotedPatterns {
		if strings.Contains(lowerQuery, strings.ToLower(pattern)) {
			return false // Trigger correction
		}
	}

	return false
}

// ensureSiteIDFilter adds numeric site_id filter if missing
func (c *Client) ensureSiteIDFilter(query string, siteID int64) string {
	upperQuery := strings.ToUpper(query)
	siteFilter := fmt.Sprintf("site_id = %d", siteID)

	// Fix quoted site_id values first
	query = c.fixQuotedSiteID(query, siteID)
	upperQuery = strings.ToUpper(query)

	if c.validateSiteIDPresent(query, siteID) {
		return query
	}

	if strings.Contains(upperQuery, "WHERE") {
		whereIndex := strings.Index(upperQuery, "WHERE")
		if whereIndex >= 0 {
			beforeWhere := query[:whereIndex+5]
			afterWhere := query[whereIndex+5:]

			if !strings.Contains(strings.ToLower(afterWhere), "site_id") {
				return fmt.Sprintf("%s %s AND%s", beforeWhere, siteFilter, afterWhere)
			}
		}
	} else if strings.Contains(upperQuery, "FROM") {
		fromMatch := regexp.MustCompile(`(?i)FROM\s+(\S+)`).FindStringIndex(query)
		if fromMatch != nil {
			remainingQuery := query[fromMatch[1]:]
			keywords := []string{"GROUP", "ORDER", "LIMIT", "HAVING", "UNION", ";"}

			insertPos := len(query)
			for _, keyword := range keywords {
				if idx := strings.Index(strings.ToUpper(remainingQuery), keyword); idx > 0 {
					insertPos = fromMatch[1] + idx
					break
				}
			}

			beforeInsert := strings.TrimSpace(query[:insertPos])
			afterInsert := query[insertPos:]
			return fmt.Sprintf("%s WHERE %s %s", beforeInsert, siteFilter, afterInsert)
		}
	}

	c.logger.WithFields(logrus.Fields{
		"query":   query,
		"site_id": siteID,
	}).Warn("Could not automatically add site_id filter")

	return query
}

// fixQuotedSiteID fixes incorrectly quoted site_id values
func (c *Client) fixQuotedSiteID(query string, siteID int64) string {
	wrongPatterns := []string{
		fmt.Sprintf("site_id = '%d'", siteID),
		fmt.Sprintf("site_id='%d'", siteID),
		fmt.Sprintf(`site_id = "%d"`, siteID),
		fmt.Sprintf(`site_id="%d"`, siteID),
	}

	correctPattern := fmt.Sprintf("site_id = %d", siteID)

	result := query
	for _, wrong := range wrongPatterns {
		result = strings.ReplaceAll(result, wrong, correctPattern)
	}

	return result
}

// ValidateConnection checks if the OpenAI API is accessible
func (c *Client) ValidateConnection(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	modelsList, err := c.client.ListModels(ctx)
	if err != nil {
		c.logger.WithError(err).Error("OpenAI connection validation failed")
		return fmt.Errorf("OpenAI connection validation failed: %w", err)
	}

	c.logger.WithField("model_count", len(modelsList.Models)).Info("OpenAI connection validated successfully")
	return nil
}
