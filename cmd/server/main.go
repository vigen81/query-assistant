package main

import (
	"context"

	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/clickhouse"
	"gitlab.smartbet.am/golang/query-assistant/internal/config"
	"gitlab.smartbet.am/golang/query-assistant/internal/handlers"
	"gitlab.smartbet.am/golang/query-assistant/internal/logger"
	"gitlab.smartbet.am/golang/query-assistant/internal/openai"
	"gitlab.smartbet.am/golang/query-assistant/internal/repository"
	"gitlab.smartbet.am/golang/query-assistant/internal/server"
	"gitlab.smartbet.am/golang/query-assistant/internal/services"

	// Import generated docs for Swagger
	_ "gitlab.smartbet.am/golang/query-assistant/docs"

	"go.uber.org/fx"
)

// @title Query Assistant API
// @version 1.0
// @description An intelligent query assistant that uses ChatGPT to generate and execute ClickHouse queries based on natural language prompts with multi-tenant support.
// @description
// @description ## Features
// @description - **Natural Language Processing**: Convert plain English prompts to ClickHouse SQL queries
// @description - **Multi-Tenant Support**: All queries are filtered by site_id for data isolation
// @description - **Schema-Aware**: Understands your database schema for accurate query generation
// @description - **Business Logic**: Built-in understanding of gaming metrics (GGR = Bet - Win)
// @description - **Query Execution**: Automatically executes generated queries with timeout protection
// @description - **Secure**: Query validation, site_id enforcement, and execution limits
// @description - **Observable**: Structured logging with Graylog integration
// @description
// @description ## Important Business Rules
// @description - **GGR Calculation**: Gross Gaming Revenue is always calculated as Total Bet - Total Win
// @description - **Site Isolation**: All queries are automatically filtered by site_id
// @description - **Archive Tables**: Queries on archive tables always include created_at filters
// @description - **RMT Tables**: Tables with 'rmt' in name use FINAL keyword for consistency
// @description
// @description ## Multi-Tenant Architecture
// @description All API endpoints require a numeric site_id parameter to ensure data isolation between different sites/tenants.
// @description The system automatically adds WHERE site_id = {your_site_id} to all generated queries.
// @description
// @description ## Authentication
// @description Most endpoints require a Bearer token in the Authorization header:
// @description Authorization: Bearer {your-jwt-token}

// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@yourcompany.com

// @host localhost:8080
// @BasePath /api/v1

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

// @tag.name query
// @tag.description Query generation and execution operations with multi-tenant support

// @tag.name schema
// @tag.description Database schema information

// @tag.name health
// @tag.description Health and readiness checks

func main() {
	serviceName := "query-assistant"

	fx.New(
		fx.Provide(func(lifecycle fx.Lifecycle) (*config.Config, error) {
			return config.Provider(lifecycle, serviceName)
		}),
		fx.Provide(func() *logrus.Logger {
			return logger.NewLogger()
		}),

		// ClickHouse
		fx.Provide(func(cfg *config.Config, logger *logrus.Logger) (*clickhouse.Client, error) {
			return clickhouse.NewClient(cfg, logger)
		}),

		// OpenAI Client
		fx.Provide(func(cfg *config.Config, logger *logrus.Logger) *openai.Client {
			return openai.NewClient(cfg, logger)
		}),

		// Repositories
		fx.Provide(func(client *clickhouse.Client, logger *logrus.Logger) *repository.SchemaRepository {
			return repository.NewSchemaRepository(client, logger)
		}),
		fx.Provide(func(client *clickhouse.Client, logger *logrus.Logger) *repository.QueryRepository {
			return repository.NewQueryRepository(client, logger)
		}),

		// Services
		fx.Provide(func(
			openaiClient *openai.Client,
			schemaRepo *repository.SchemaRepository,
			queryRepo *repository.QueryRepository,
			logger *logrus.Logger,
		) *services.QueryService {
			return services.NewQueryService(openaiClient, schemaRepo, queryRepo, logger)
		}),

		// Handlers
		fx.Provide(func(
			queryService *services.QueryService,
			logger *logrus.Logger,
		) *handlers.QueryHandler {
			return handlers.NewQueryHandler(queryService, logger)
		}),
		fx.Provide(func(
			schemaRepo *repository.SchemaRepository,
			logger *logrus.Logger,
		) *handlers.SchemaHandler {
			return handlers.NewSchemaHandler(schemaRepo, logger)
		}),
		fx.Provide(func(logger *logrus.Logger) *handlers.HealthHandler {
			return handlers.NewHealthHandler(logger)
		}),

		// Server
		fx.Provide(func(
			cfg *config.Config,
			queryHandler *handlers.QueryHandler,
			schemaHandler *handlers.SchemaHandler,
			healthHandler *handlers.HealthHandler,
			logger *logrus.Logger,
		) *server.FiberServer {
			return server.NewFiberServer(cfg, queryHandler, schemaHandler, healthHandler, logger)
		}),

		// Lifecycle
		fx.Invoke(func(
			lifecycle fx.Lifecycle,
			fiberServer *server.FiberServer,
			schemaRepo *repository.SchemaRepository,
			logger *logrus.Logger,
		) {
			lifecycle.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					logger.Info("Starting query assistant application")

					schemaRepo.LogDatabaseSchema(ctx)

					// Start HTTP server in goroutine
					go func() {
						if err := fiberServer.Start(":8080"); err != nil {
							logger.WithError(err).Fatal("Failed to start server")
						}
					}()

					logger.Info("Query assistant started successfully")
					return nil
				},
				OnStop: func(ctx context.Context) error {
					logger.Info("Stopping query assistant application")

					if err := fiberServer.Shutdown(ctx); err != nil {
						logger.WithError(err).Error("Error shutting down server")
					}

					logger.Info("Query assistant stopped")
					return nil
				},
			})
		}),
	).Run()
}
