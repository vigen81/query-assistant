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
// @description An intelligent query assistant that uses ChatGPT to generate and execute ClickHouse queries based on natural language prompts.
// @description
// @description ## Features
// @description - **Natural Language Processing**: Convert plain English prompts to ClickHouse SQL queries
// @description - **Schema-Aware**: Understands your database schema for accurate query generation
// @description - **Query Execution**: Automatically executes generated queries with timeout protection
// @description - **Secure**: Query validation and execution limits
// @description - **Observable**: Structured logging with Graylog integration
// @description
// @description ## How it works
// @description 1. User submits a natural language prompt about their data
// @description 2. System sends the prompt along with schema information to ChatGPT
// @description 3. ChatGPT generates an appropriate ClickHouse query
// @description 4. System validates and executes the query with timeout protection
// @description 5. Results are formatted and returned to the user

// @termsOfService http://swagger.io/terms/

// @host localhost:8080
// @BasePath /api/v1

// @tag.name query
// @tag.description Query generation and execution operations

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
			logger *logrus.Logger,
		) {
			lifecycle.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					logger.Info("Starting query assistant application")

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
