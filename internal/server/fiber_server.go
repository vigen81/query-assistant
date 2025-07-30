package server

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/gofiber/swagger"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/config"
	"gitlab.smartbet.am/golang/query-assistant/internal/handlers"
	"gitlab.smartbet.am/golang/query-assistant/internal/middleware"
)

type FiberServer struct {
	app           *fiber.App
	config        *config.Config
	queryHandler  *handlers.QueryHandler
	schemaHandler *handlers.SchemaHandler
	healthHandler *handlers.HealthHandler
	logger        *logrus.Logger
}

func NewFiberServer(
	config *config.Config,
	queryHandler *handlers.QueryHandler,
	schemaHandler *handlers.SchemaHandler,
	healthHandler *handlers.HealthHandler,
	logger *logrus.Logger,
) *FiberServer {
	app := fiber.New(fiber.Config{
		ReadTimeout:  config.GetServerReadTimeout(),
		WriteTimeout: config.GetServerWriteTimeout(),
		IdleTimeout:  config.GetServerIdleTimeout(),
		ErrorHandler: customErrorHandler,
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(fiberlogger.New(fiberlogger.Config{
		Format: "[${time}] ${status} - ${latency} ${method} ${path}\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	server := &FiberServer{
		app:           app,
		config:        config,
		queryHandler:  queryHandler,
		schemaHandler: schemaHandler,
		healthHandler: healthHandler,
		logger:        logger,
	}

	server.setupRoutes()
	return server
}

func (s *FiberServer) setupRoutes() {
	// Root level health check endpoints
	s.app.Get("/health", s.healthHandler.HealthCheck)
	s.app.Get("/ready", s.healthHandler.ReadinessCheck)
	s.app.Get("/live", s.healthHandler.LivenessCheck)

	// Swagger documentation
	if s.config.Swagger.Enabled {
		s.app.Get("/swagger/*", swagger.HandlerDefault)
	}

	// API v1 routes
	v1 := s.app.Group("/api/v1")

	// Apply auth middleware unless skip_auth is true
	if !s.config.Auth.SkipAuth {
		v1.Use(middleware.AuthMiddleware(s.config))
	}

	// Health endpoints also under /api/v1
	v1.Get("/health", s.healthHandler.HealthCheck)
	v1.Get("/ready", s.healthHandler.ReadinessCheck)
	v1.Get("/live", s.healthHandler.LivenessCheck)

	// Query routes
	query := v1.Group("/query")
	query.Post("/execute", s.queryHandler.ExecuteQuery)
	query.Post("/validate", s.queryHandler.ValidateQuery)
	query.Post("/generate", s.queryHandler.GenerateQuery)

	// Schema routes
	schema := v1.Group("/schema")
	schema.Get("/", s.schemaHandler.GetDatabaseSchema)
	schema.Get("/table/:table", s.schemaHandler.GetTableSchema)
	schema.Post("/refresh", s.schemaHandler.RefreshSchema)

	// Catch-all route for undefined endpoints
	s.app.Use("*", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":     "Endpoint not found",
			"code":      "NOT_FOUND",
			"path":      c.Path(),
			"method":    c.Method(),
			"timestamp": time.Now(),
		})
	})
}

func (s *FiberServer) Start(addr string) error {
	s.logger.WithField("address", addr).Info("Starting Fiber server")
	return s.app.Listen(addr)
}

func (s *FiberServer) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down Fiber server")
	return s.app.ShutdownWithContext(ctx)
}

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal Server Error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	return c.Status(code).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": c.Locals("requestid"),
		"timestamp":  time.Now(),
	})
}
