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
	// Get API key from environment variable first, then fall back to config
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = cfg.OpenAI.APIKey
	}

	// Log API key info (masked for security)
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

// GenerateQuery generates a ClickHouse query from a natural language prompt with numeric site_id filtering
func (c *Client) GenerateQuery(ctx context.Context, prompt string, schemaInfo *models.SchemaInfo, siteID int64) (string, error) {
	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, c.config.GetOpenAITimeout())
	defer cancel()

	// Build the system message with schema context and site_id requirements
	systemMessage := c.buildSystemMessageWithSiteID(schemaInfo, siteID)

	// Build the user message with site_id context and ALL original rules
	userMessage := fmt.Sprintf(`Generate a ClickHouse SQL query for the following request: "%s"

IMPORTANT RULES:
1. Generate ONLY a SELECT statement - never use CREATE, DROP, INSERT, UPDATE, DELETE, or any other DDL/DML operations
2. Return ONLY the SQL query itself - no markdown formatting, no explanations, no comments
3. Do not wrap the query in backticks or code blocks
4. Include appropriate WHERE clauses for efficiency
5. Use proper ClickHouse functions and syntax
6. Add LIMIT clause (default to 100 rows if not specified)
7. The query must be ready to execute as-is
8. ignore _peerdb_synced_at  _peerdb_is_deleted  _peerdb_version fields AT all
9. use FINAL for rmt table
10. add created_at range to tables if possible
11. always add created_at on table bh_transaction_main_archive and bh_payment_archive
12. ALWAYS include a WHERE clause that filters by site_id = %d (numeric, no quotes)
13. If the query involves JOINs, ensure ALL joined tables are filtered by site_id = %d
14. GGR (Gross Gaming Revenue) = Total Bet - Total Win (always calculate GGR this way)

SITE FILTERING RULES:
- Every table referenced must have: WHERE site_id = %d (or AND site_id = %d if other conditions exist)
- For JOINs: table1.site_id = %d AND table2.site_id = %d
- For subqueries: Each subquery must also filter by site_id = %d
- For CTEs (WITH clauses): Each CTE must filter by site_id = %d
- site_id is a numeric field (UInt64 or Int64), do NOT use quotes around the value

BUSINESS LOGIC RULES:
- GGR (Gross Gaming Revenue) MUST be calculated as: Total Bet - Total Win
- When calculating GGR, use: SUM(bet_amount) - SUM(win_amount) or equivalent
- Never calculate GGR differently

Example format:
SELECT column1, column2 
FROM table FINAL 
WHERE site_id = %d AND created_at >= today() - 30 AND other_conditions 
LIMIT 100`, prompt, siteID, siteID, siteID, siteID, siteID, siteID, siteID, siteID, siteID)

	// Get model from environment or config
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
	}).Debug("Requesting query generation from OpenAI with numeric site_id filter")

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

	// Validate that site_id filtering is present
	if !c.validateSiteIDPresent(generatedQuery, siteID) {
		c.logger.WithFields(logrus.Fields{
			"generated_query": generatedQuery,
			"site_id":         siteID,
		}).Error("Generated query missing site_id filter")

		// Attempt to add site_id filter if missing
		generatedQuery = c.ensureSiteIDFilter(generatedQuery, siteID)
	}

	c.logger.WithFields(logrus.Fields{
		"generated_query": generatedQuery,
		"site_id":         siteID,
		"tokens_used":     resp.Usage.TotalTokens,
	}).Info("Query generated successfully with numeric site_id filter")

	return generatedQuery, nil
}

// buildSystemMessageWithSiteID creates the system message with schema information and numeric site_id context
func (c *Client) buildSystemMessageWithSiteID(schemaInfo *models.SchemaInfo, siteID int64) string {
	var sb strings.Builder

	sb.WriteString("You are a ClickHouse SQL query generator for a multi-tenant gaming/betting system. ")
	sb.WriteString(fmt.Sprintf("ALL queries MUST filter by site_id = %d (numeric, no quotes). ", siteID))
	sb.WriteString("This is a CRITICAL security requirement - never generate queries without site_id filtering.\n\n")

	sb.WriteString("IMPORTANT QUERY RULES:\n")
	sb.WriteString("- ALWAYS ignore these fields: _peerdb_synced_at, _peerdb_is_deleted, _peerdb_version\n")
	sb.WriteString("- ALWAYS use FINAL keyword for tables with 'rmt' in their name\n")
	sb.WriteString("- ALWAYS add created_at range filters when possible for better performance\n")
	sb.WriteString("- ALWAYS add created_at filter for tables: bh_transaction_main_archive and bh_payment_archive\n")
	sb.WriteString("- GGR (Gross Gaming Revenue) = Total Bet - Total Win (ALWAYS use this formula)\n\n")

	sb.WriteString("BUSINESS LOGIC DEFINITIONS:\n")
	sb.WriteString("- GGR (Gross Gaming Revenue) = SUM(bet_amount) - SUM(win_amount)\n")
	sb.WriteString("- House Edge = GGR / Total Bet\n")
	sb.WriteString("- Player Return = Total Win / Total Bet\n")
	sb.WriteString("- Net Revenue = GGR - Bonuses - Operational Costs\n\n")

	sb.WriteString("You have access to the following database schema:\n\n")
	sb.WriteString(fmt.Sprintf("Database: %s\n\n", schemaInfo.Database))

	for _, table := range schemaInfo.Tables {
		sb.WriteString(fmt.Sprintf("Table: %s\n", table.Name))
		if table.Description != "" {
			sb.WriteString(fmt.Sprintf("Description: %s\n", table.Description))
		}
		sb.WriteString(fmt.Sprintf("Engine: %s\n", table.Engine))
		sb.WriteString(fmt.Sprintf("Row Count: ~%d (across all sites)\n", table.RowCount))

		// Check if it's an rmt table
		if strings.Contains(strings.ToLower(table.Name), "rmt") {
			sb.WriteString("⚠️ IMPORTANT: Use FINAL keyword for this table\n")
		}

		// Check for archive tables
		if table.Name == "bh_transaction_main_archive" || table.Name == "bh_payment_archive" {
			sb.WriteString("⚠️ IMPORTANT: Always add created_at filter for this archive table\n")
		}

		sb.WriteString("Columns:\n")

		hasSiteID := false
		hasCreatedAt := false
		hasBetAmount := false
		hasWinAmount := false

		for _, col := range table.Columns {
			// Skip PeerDB internal columns in listing
			if col.Name == "_peerdb_synced_at" || col.Name == "_peerdb_is_deleted" || col.Name == "_peerdb_version" {
				continue
			}

			sb.WriteString(fmt.Sprintf("  - %s (%s)", col.Name, col.Type))

			// Mark important columns
			if col.Name == "site_id" {
				sb.WriteString(" [REQUIRED FILTER COLUMN - NUMERIC]")
				hasSiteID = true
			}
			if col.Name == "created_at" {
				sb.WriteString(" [USE FOR DATE RANGE FILTERING]")
				hasCreatedAt = true
			}
			if strings.Contains(strings.ToLower(col.Name), "bet") && strings.Contains(strings.ToLower(col.Name), "amount") {
				sb.WriteString(" [BET AMOUNT - Use for GGR calculation]")
				hasBetAmount = true
			}
			if strings.Contains(strings.ToLower(col.Name), "win") && strings.Contains(strings.ToLower(col.Name), "amount") {
				sb.WriteString(" [WIN AMOUNT - Use for GGR calculation]")
				hasWinAmount = true
			}

			if col.Description != "" {
				sb.WriteString(fmt.Sprintf(" - %s", col.Description))
			}
			if col.IsNullable {
				sb.WriteString(" [nullable]")
			}
			sb.WriteString("\n")
		}

		// Add table-specific notes
		if hasSiteID {
			sb.WriteString(fmt.Sprintf("  ⚠️ This table MUST be filtered by site_id = %d (numeric, no quotes)\n", siteID))
		}
		if hasCreatedAt {
			sb.WriteString("  ℹ️ Use created_at for date range filtering to improve performance\n")
		}
		if hasBetAmount && hasWinAmount {
			sb.WriteString("  💰 This table can be used for GGR calculation: SUM(bet_amount) - SUM(win_amount)\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\nCRITICAL Multi-tenant Requirements:\n")
	sb.WriteString(fmt.Sprintf("- ALWAYS filter every table by site_id = %d (numeric value, NO quotes)\n", siteID))
	sb.WriteString("- site_id is a numeric field (UInt64/Int64) - do NOT wrap in quotes\n")
	sb.WriteString("- For JOINs, filter BOTH tables by site_id\n")
	sb.WriteString("- For subqueries, filter each subquery by site_id\n")
	sb.WriteString("- For CTEs, filter each CTE by site_id\n")
	sb.WriteString("- NEVER generate queries that could access data from other sites\n\n")

	sb.WriteString("Important ClickHouse-specific considerations:\n")
	sb.WriteString("- Use FINAL for ReplacingMergeTree tables (tables with 'rmt' in name)\n")
	sb.WriteString("- Use appropriate date/time functions (toDate, toDateTime, etc.)\n")
	sb.WriteString("- Add created_at filters for archive tables\n")
	sb.WriteString("- Ignore PeerDB sync columns (_peerdb_synced_at, _peerdb_is_deleted, _peerdb_version)\n")
	sb.WriteString("- Consider using aggregation functions with GROUP BY\n")
	sb.WriteString("- Use LIMIT to control result size\n")
	sb.WriteString("- Optimize for columnar storage patterns\n")
	sb.WriteString("- Use appropriate JOIN types and conditions\n")
	sb.WriteString("- For GGR calculations, always use: SUM(bet_amount) - SUM(win_amount)\n")

	return sb.String()
}

// buildSystemMessage creates the system message with schema information (legacy, without site_id)
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

// validateSiteIDPresent checks if the query contains proper numeric site_id filtering
func (c *Client) validateSiteIDPresent(query string, siteID int64) bool {
	lowerQuery := strings.ToLower(query)

	// Check for site_id column reference
	if !strings.Contains(lowerQuery, "site_id") {
		return false
	}

	// Check for the specific numeric site_id value (without quotes)
	expectedPatterns := []string{
		fmt.Sprintf("site_id = %d", siteID),
		fmt.Sprintf("site_id=%d", siteID),
		fmt.Sprintf("site_id = %d ", siteID),
		fmt.Sprintf("site_id=%d ", siteID),
	}

	for _, pattern := range expectedPatterns {
		if strings.Contains(lowerQuery, strings.ToLower(pattern)) {
			return true
		}
	}

	// Check if someone mistakenly used quotes (we should fix this)
	quotedPatterns := []string{
		fmt.Sprintf("site_id = '%d'", siteID),
		fmt.Sprintf("site_id='%d'", siteID),
	}

	for _, pattern := range quotedPatterns {
		if strings.Contains(lowerQuery, strings.ToLower(pattern)) {
			c.logger.Warn("Found site_id with quotes, this should be numeric")
			return false // Return false to trigger correction
		}
	}

	return false
}

// ensureSiteIDFilter attempts to add numeric site_id filter if missing
func (c *Client) ensureSiteIDFilter(query string, siteID int64) string {
	upperQuery := strings.ToUpper(query)
	siteFilter := fmt.Sprintf("site_id = %d", siteID) // No quotes for numeric

	// First, fix any quoted site_id values
	query = c.fixQuotedSiteID(query, siteID)
	upperQuery = strings.ToUpper(query)

	// Check if site_id is now present
	if c.validateSiteIDPresent(query, siteID) {
		return query
	}

	// Add site_id filter if missing
	if strings.Contains(upperQuery, "WHERE") {
		// Query has WHERE clause, add site_id as first condition
		whereIndex := strings.Index(upperQuery, "WHERE")
		if whereIndex >= 0 {
			beforeWhere := query[:whereIndex+5] // Include "WHERE"
			afterWhere := query[whereIndex+5:]

			// Check if site_id already exists
			if !strings.Contains(strings.ToLower(afterWhere), "site_id") {
				return fmt.Sprintf("%s %s AND%s", beforeWhere, siteFilter, afterWhere)
			}
		}
	} else if strings.Contains(upperQuery, "FROM") {
		// No WHERE clause, add one after FROM
		fromMatch := regexp.MustCompile(`(?i)FROM\s+(\S+)`).FindStringIndex(query)
		if fromMatch != nil {
			// Find next SQL keyword or end of query
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

// fixQuotedSiteID fixes incorrectly quoted site_id values
func (c *Client) fixQuotedSiteID(query string, siteID int64) string {
	// Fix quoted site_id values
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
	// Simple test to check API key validity
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Test with a simple model list request
	modelsList, err := c.client.ListModels(ctx)
	if err != nil {
		c.logger.WithError(err).Error("OpenAI connection validation failed")
		return fmt.Errorf("OpenAI connection validation failed: %w", err)
	}

	c.logger.WithField("model_count", len(modelsList.Models)).Info("OpenAI connection validated successfully")
	return nil
}
