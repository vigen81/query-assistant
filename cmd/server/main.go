package main

import (
	"context"
	"os"

	openaisdk "github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/clickhouse"
	"gitlab.smartbet.am/golang/query-assistant/internal/config"
	"gitlab.smartbet.am/golang/query-assistant/internal/handlers"
	"gitlab.smartbet.am/golang/query-assistant/internal/imagegen"
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
// @description - **Banner Generation**: Async AI-powered marketing banner image generation via DALL-E 3
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
// @description
// @description ## Banner Image Generation
// @description Banner generation is asynchronous. POST to /api/v1/banner/generate to start a job,
// @description then poll GET /api/v1/banner/generate/{id} until status is "completed" or "failed".
// @description Default variant count is 4 images per request (configurable via banner.variant_count in config).

// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@yourcompany.com

// @host dev.smartbet.live
// @BasePath /query-assistant/api/v1

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

// @tag.name query
// @tag.description Query generation and execution operations with multi-tenant support

// @tag.name schema
// @tag.description Database schema information

// @tag.name health
// @tag.description Health and readiness checks

// @tag.name banner
// @tag.description Async AI banner image generation via DALL-E 3. POST to start, GET to poll status.

func main() {
	serviceName := "query-assistant"

	fx.New(
		// ----------------------------------------------------------------
		// Config & Logger
		// ----------------------------------------------------------------
		fx.Provide(func(lifecycle fx.Lifecycle) (*config.Config, error) {
			return config.Provider(lifecycle, serviceName)
		}),
		fx.Provide(func() *logrus.Logger {
			return logger.NewLogger()
		}),

		// ----------------------------------------------------------------
		// ClickHouse
		// ----------------------------------------------------------------
		fx.Provide(func(cfg *config.Config, log *logrus.Logger) (*clickhouse.Client, error) {
			return clickhouse.NewClient(cfg, log)
		}),

		// ----------------------------------------------------------------
		// OpenAI (query generation)
		// ----------------------------------------------------------------
		fx.Provide(func(cfg *config.Config, log *logrus.Logger) *openai.Client {
			return openai.NewClient(cfg, log)
		}),

		// ----------------------------------------------------------------
		// Image generation
		// ----------------------------------------------------------------

		// Low-level sashabaranov OpenAI client shared by the image adapter.
		fx.Provide(func(cfg *config.Config) *openaisdk.Client {
			apiKey := os.Getenv("OPENAI_API_KEY")
			if apiKey == "" {
				apiKey = cfg.OpenAI.APIKey
			}
			return openaisdk.NewClient(apiKey)
		}),

		// OpenAI DALL-E 3 adapter (implements imagegen.Provider).
		fx.Provide(func(client *openaisdk.Client, log *logrus.Logger) imagegen.Provider {
			return imagegen.NewOpenAIAdapter(client, log)
		}),

		// Prompt builder (versioned, language-aware).
		fx.Provide(imagegen.NewPromptBuilder),

		// In-memory generation store.
		fx.Provide(imagegen.NewGenerationStore),

		// ----------------------------------------------------------------
		// Repositories
		// ----------------------------------------------------------------
		fx.Provide(func(client *clickhouse.Client, log *logrus.Logger) *repository.SchemaRepository {
			return repository.NewSchemaRepository(client, log)
		}),
		fx.Provide(func(client *clickhouse.Client, log *logrus.Logger) *repository.QueryRepository {
			return repository.NewQueryRepository(client, log)
		}),

		// ----------------------------------------------------------------
		// Services
		// ----------------------------------------------------------------
		fx.Provide(func(
			oc *openai.Client,
			sr *repository.SchemaRepository,
			qr *repository.QueryRepository,
			log *logrus.Logger,
		) *services.QueryService {
			return services.NewQueryService(oc, sr, qr, log)
		}),

		fx.Provide(func(
			provider imagegen.Provider,
			pb *imagegen.PromptBuilder,
			store *imagegen.GenerationStore,
			cfg *config.Config,
			log *logrus.Logger,
		) *services.BannerService {
			return services.NewBannerService(
				provider,
				pb,
				store,
				&services.BannerServiceConfig{VariantCount: cfg.Banner.VariantCount},
				log,
			)
		}),

		// ----------------------------------------------------------------
		// Handlers
		// ----------------------------------------------------------------
		fx.Provide(func(qs *services.QueryService, log *logrus.Logger) *handlers.QueryHandler {
			return handlers.NewQueryHandler(qs, log)
		}),
		fx.Provide(func(sr *repository.SchemaRepository, log *logrus.Logger) *handlers.SchemaHandler {
			return handlers.NewSchemaHandler(sr, log)
		}),
		fx.Provide(func(log *logrus.Logger) *handlers.HealthHandler {
			return handlers.NewHealthHandler(log)
		}),
		fx.Provide(func(bs *services.BannerService, log *logrus.Logger) *handlers.BannerHandler {
			return handlers.NewBannerHandler(bs, log)
		}),

		// ----------------------------------------------------------------
		// HTTP server
		// ----------------------------------------------------------------
		fx.Provide(func(
			cfg *config.Config,
			qh *handlers.QueryHandler,
			sh *handlers.SchemaHandler,
			hh *handlers.HealthHandler,
			bh *handlers.BannerHandler,
			log *logrus.Logger,
		) *server.FiberServer {
			return server.NewFiberServer(cfg, qh, sh, hh, bh, log)
		}),

		// ----------------------------------------------------------------
		// Lifecycle
		// ----------------------------------------------------------------
		fx.Invoke(func(
			lifecycle fx.Lifecycle,
			fiberServer *server.FiberServer,
			log *logrus.Logger,
		) {
			lifecycle.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					log.Info("Starting query-assistant application")
					go func() {
						if err := fiberServer.Start(":8080"); err != nil {
							log.WithError(err).Fatal("Failed to start server")
						}
					}()
					log.Info("Query-assistant started successfully")
					return nil
				},
				OnStop: func(ctx context.Context) error {
					log.Info("Stopping query-assistant application")
					if err := fiberServer.Shutdown(ctx); err != nil {
						log.WithError(err).Error("Error shutting down server")
					}
					log.Info("Query-assistant stopped")
					return nil
				},
			})
		}),
	).Run()
}
