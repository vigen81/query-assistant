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

// GenerateQuery generates a ClickHouse query from a natural language prompt
// using the Semantic Dictionary as the single source of truth.
func (c *Client) GenerateQuery(ctx context.Context, prompt string, siteID int64) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.config.GetOpenAITimeout())
	defer cancel()

	// Build messages from semantic dictionary
	messages := c.buildSemanticDictionaryMessages(prompt, siteID)

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = c.config.OpenAI.Model
	}

	c.logger.WithFields(logrus.Fields{
		"prompt":  prompt,
		"site_id": siteID,
		"model":   model,
	}).Debug("Requesting query generation from OpenAI")

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

	// Strip markdown fences if the model wraps output
	generatedQuery = stripMarkdownFences(generatedQuery)

	// Validate site_id filtering
	if !c.validateSiteIDPresent(generatedQuery, siteID) {
		c.logger.WithFields(logrus.Fields{
			"generated_query": generatedQuery,
			"site_id":         siteID,
		}).Error("Generated query missing site_id filter")
		generatedQuery = c.ensureSiteIDFilter(generatedQuery, siteID)
	}

	c.logger.WithFields(logrus.Fields{
		"generated_query":   generatedQuery,
		"site_id":           siteID,
		"tokens_used":       resp.Usage.TotalTokens,
		"prompt_tokens":     resp.Usage.PromptTokens,
		"completion_tokens": resp.Usage.CompletionTokens,
	}).Info("Query generated successfully")

	return generatedQuery, nil
}

// buildSemanticDictionaryMessages constructs the OpenAI messages using the
// semantic dictionary constants selected for the current environment (POD_ENV).
func (c *Client) buildSemanticDictionaryMessages(prompt string, siteID int64) []openai.ChatCompletionMessage {
	env := os.Getenv("POD_ENV")
	c.logger.WithField("POD_ENV", env).Info("Building semantic dictionary messages")

	// 1. System prompt — behavioural rules
	systemPrompt := GetSystemPrompt()

	// 2. Semantic dictionary — tables, metrics, joins, aliases, presets
	semanticDictionary := GetSemanticDictionary()

	// 3. DDL schema — column-level types
	ddlSchema := GetDDLSchema()

	// Inject site_id into the system prompt
	systemPrompt = strings.ReplaceAll(systemPrompt, "{site_id}", fmt.Sprintf("%d", siteID))

	// Build the three context messages + user prompt
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: semanticDictionary,
		},
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: ddlSchema,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}

	return messages
}

// stripMarkdownFences removes ```sql ... ``` wrapping if present.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Remove opening fence (```sql or ```)
		if idx := strings.Index(s, "\n"); idx > 0 {
			s = s[idx+1:]
		}
		// Remove closing fence
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
		s = strings.TrimSpace(s)
	}
	return s
}

// ---------- site_id validation & safety (unchanged) ----------

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

	// Detect incorrectly quoted site_id
	quotedPatterns := []string{
		fmt.Sprintf("site_id = '%d'", siteID),
		fmt.Sprintf("site_id='%d'", siteID),
	}
	for _, pattern := range quotedPatterns {
		if strings.Contains(lowerQuery, strings.ToLower(pattern)) {
			c.logger.Warn("Found site_id with quotes, this should be numeric")
			return false
		}
	}

	return false
}

func (c *Client) ensureSiteIDFilter(query string, siteID int64) string {
	upperQuery := strings.ToUpper(query)
	siteFilter := fmt.Sprintf("site_id = %d", siteID)

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
	}).Warn("Could not automatically add site_id filter, returning original query")

	return query
}

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

// ValidateConnection checks if the OpenAI API is accessible.
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
