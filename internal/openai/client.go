package openai

import (
	"context"
	"fmt"
	"os"
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

	// Build messages in the correct order:
	// 1. System Prompt (instructions)
	// 2. Dictionary (semantic mappings)
	// 3. DDL (database schema)
	// 4. User Prompt (actual query request)

	systemMessage := c.buildSystemPrompt(siteID)
	dictionaryMessage := c.buildDictionaryMessage()
	ddlMessage := c.buildDDLMessage()
	userMessage := c.buildUserMessage(prompt, siteID)

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
			Content: dictionaryMessage,
		},
		{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "I have loaded the Semantic Dictionary. I understand all table mappings, metrics, dimensions, joins, and semantic aliases. Ready to generate SQL queries.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: ddlMessage,
		},
		{
			Role:    openai.ChatMessageRoleAssistant,
			Content: "I have loaded the Database Schema (DDL). I understand all column names, data types, and table structures. Ready for your query.",
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
	}).Debug("Requesting query generation from OpenAI")

	// Log full request for debugging (set LOG_OPENAI_REQUEST=true to enable)
	if os.Getenv("LOG_OPENAI_REQUEST") == "true" {
		c.logger.WithFields(logrus.Fields{
			"system_prompt_length": len(systemMessage),
			"dictionary_length":    len(dictionaryMessage),
			"ddl_length":           len(ddlMessage),
			"user_message":         userMessage,
			"model":                model,
			"max_tokens":           c.config.OpenAI.MaxTokens,
			"temperature":          c.config.OpenAI.Temperature,
		}).Debug("OpenAI request summary")
	}

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

	// Check if this is a predefined response (not SQL)
	if c.isPredefinedResponse(generatedQuery) {
		c.logger.WithFields(logrus.Fields{
			"response": generatedQuery,
			"site_id":  siteID,
		}).Info("Received predefined response (clarification or off-topic)")
		return generatedQuery, nil
	}

	// Clean up the query
	generatedQuery = cleanQuery(generatedQuery)

	// Validate that the query contains the site_id filter
	if !strings.Contains(generatedQuery, fmt.Sprintf("site_id = %d", siteID)) &&
		!strings.Contains(generatedQuery, fmt.Sprintf("site_id=%d", siteID)) {
		c.logger.WithFields(logrus.Fields{
			"generated_query": generatedQuery,
			"site_id":         siteID,
		}).Warn("Generated query may be missing site_id filter")
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

// buildSystemPrompt creates the system prompt with core instructions
func (c *Client) buildSystemPrompt(siteID int64) string {
	return fmt.Sprintf(`%s

---

## CURRENT QUERY CONTEXT

**Site ID**: %d
**CRITICAL**: All queries MUST filter by site_id = %d (numeric, no quotes)
`, SystemPrompt, siteID, siteID)
}

// buildDictionaryMessage creates the dictionary context message
func (c *Client) buildDictionaryMessage() string {
	return fmt.Sprintf(`Please load this Semantic Dictionary for reference:

%s`, SemanticDictionary)
}

// buildDDLMessage creates the DDL schema context message
func (c *Client) buildDDLMessage() string {
	return fmt.Sprintf(`Please load this Database Schema (DDL) for reference:

%s`, DDLSchema)
}

// buildUserMessage creates the user query message
func (c *Client) buildUserMessage(prompt string, siteID int64) string {
	return fmt.Sprintf(`Generate a ClickHouse SQL query for: "%s"

REQUIREMENTS:
- site_id = %d (MANDATORY)
- Use physical table names (bh_transaction_main_archive, bh_payment_archive, m_client)
- Apply default filters (is_test=0, is_rollback=0, status IN (1,2))
- Include LIMIT 1000
- For player-level queries: SELECT a.client_id, m.username (NOT m.client_id!)
- For bh_payment_archive: use FINAL keyword (FROM bh_payment_archive AS pa FINAL)

OUTPUT: Raw SQL only, no markdown, no explanations.`, prompt, siteID)
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

// cleanQuery removes markdown formatting and extra whitespace from generated queries
func cleanQuery(query string) string {
	// Remove markdown code blocks
	query = strings.TrimPrefix(query, "```sql")
	query = strings.TrimPrefix(query, "```SQL")
	query = strings.TrimPrefix(query, "```")
	query = strings.TrimSuffix(query, "```")

	// Remove any leading/trailing whitespace
	query = strings.TrimSpace(query)

	// Normalize internal whitespace
	lines := strings.Split(query, "\n")
	var cleanedLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}
	query = strings.Join(cleanedLines, "\n")

	return query
}

// ValidateQuery performs basic validation on a generated query
func (c *Client) ValidateQuery(query string, siteID int64) error {
	// Check for required site_id filter
	if !strings.Contains(query, fmt.Sprintf("site_id = %d", siteID)) &&
		!strings.Contains(query, fmt.Sprintf("site_id=%d", siteID)) {
		return fmt.Errorf("query must contain site_id = %d filter", siteID)
	}

	// Check for forbidden operations
	forbiddenOps := []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE", "TRUNCATE", "GRANT", "REVOKE"}
	upperQuery := strings.ToUpper(query)
	for _, op := range forbiddenOps {
		if strings.Contains(upperQuery, op) {
			return fmt.Errorf("query contains forbidden operation: %s", op)
		}
	}

	// Check for LIMIT clause
	if !strings.Contains(upperQuery, "LIMIT") {
		return fmt.Errorf("query must contain LIMIT clause")
	}

	return nil
}

// HealthCheck verifies the OpenAI client is working
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: c.config.OpenAI.Model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Reply with OK",
				},
			},
			MaxTokens: 5,
		},
	)

	return err
}
