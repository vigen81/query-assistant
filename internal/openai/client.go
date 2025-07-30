package openai

import (
	"context"
	"fmt"
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
	client := openai.NewClient(cfg.OpenAI.APIKey)

	return &Client{
		client: client,
		config: cfg,
		logger: logger,
	}
}

// GenerateQuery generates a ClickHouse query from a natural language prompt
func (c *Client) GenerateQuery(ctx context.Context, prompt string, schemaInfo *models.SchemaInfo) (string, error) {
	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, c.config.GetOpenAITimeout())
	defer cancel()

	// Build the system message with schema context
	systemMessage := c.buildSystemMessage(schemaInfo)

	// Build the user message
	userMessage := fmt.Sprintf(`Generate a ClickHouse SQL query for the following request: "%s"

Rules:
1. Use only SELECT statements
2. Include appropriate WHERE clauses for efficiency
3. Use proper ClickHouse functions and syntax
4. Limit results appropriately (default to 100 rows if not specified)
5. Return ONLY the SQL query, no explanations or markdown
6. Ensure the query is optimized for ClickHouse`, prompt)

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
		"prompt": prompt,
		"model":  c.config.OpenAI.Model,
	}).Debug("Requesting query generation from OpenAI")

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:       c.config.OpenAI.Model,
			Messages:    messages,
			MaxTokens:   c.config.OpenAI.MaxTokens,
			Temperature: c.config.OpenAI.Temperature,
		},
	)

	if err != nil {
		return "", fmt.Errorf("failed to generate query: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	generatedQuery := strings.TrimSpace(resp.Choices[0].Message.Content)

	c.logger.WithFields(logrus.Fields{
		"generated_query": generatedQuery,
		"tokens_used":     resp.Usage.TotalTokens,
	}).Info("Query generated successfully")

	return generatedQuery, nil
}

// buildSystemMessage creates the system message with schema information
func (c *Client) buildSystemMessage(schemaInfo *models.SchemaInfo) string {
	var sb strings.Builder

	sb.WriteString("You are a ClickHouse SQL query generator. ")
	sb.WriteString("You have access to the following database schema:\n\n")
	sb.WriteString(fmt.Sprintf("Database: %s\n\n", schemaInfo.Database))

	for _, table := range schemaInfo.Tables {
		sb.WriteString(fmt.Sprintf("Table: %s\n", table.Name))
		if table.Description != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", table.Description))
		}
		sb.WriteString(fmt.Sprintf("Engine: %s\n", table.Engine))
		sb.WriteString(fmt.Sprintf("Row Count: ~%d\n", table.RowCount))
		sb.WriteString("Columns:\n")

		for _, col := range table.Columns {
			sb.WriteString(fmt.Sprintf("  - %s (%s)", col.Name, col.Type))
			if col.Description != "" {
				sb.WriteString(fmt.Sprintf(" - %s", col.Description))
			}
			if col.IsNullable {
				sb.WriteString(" [nullable]")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\nImportant ClickHouse-specific considerations:\n")
	sb.WriteString("- Use appropriate date/time functions (toDate, toDateTime, etc.)\n")
	sb.WriteString("- Consider using aggregation functions with GROUP BY\n")
	sb.WriteString("- Use LIMIT to control result size\n")
	sb.WriteString("- Optimize for columnar storage patterns\n")
	sb.WriteString("- Use appropriate JOIN types and conditions\n")

	return sb.String()
}

// ValidateConnection checks if the OpenAI API is accessible
func (c *Client) ValidateConnection(ctx context.Context) error {
	// Simple test to check API key validity
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := c.client.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("OpenAI connection validation failed: %w", err)
	}

	return nil
}
