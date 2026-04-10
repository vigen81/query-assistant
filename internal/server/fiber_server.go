package server

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	_ "github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/gofiber/swagger"
	"github.com/sirupsen/logrus"
	"gitlab.smartbet.am/golang/query-assistant/internal/config"
	"gitlab.smartbet.am/golang/query-assistant/internal/handlers"
)

type FiberServer struct {
	app           *fiber.App
	config        *config.Config
	queryHandler  *handlers.QueryHandler
	schemaHandler *handlers.SchemaHandler
	healthHandler *handlers.HealthHandler
	bannerHandler *handlers.BannerHandler
	logger        *logrus.Logger
}

func NewFiberServer(
	cfg *config.Config,
	queryHandler *handlers.QueryHandler,
	schemaHandler *handlers.SchemaHandler,
	healthHandler *handlers.HealthHandler,
	bannerHandler *handlers.BannerHandler,
	logger *logrus.Logger,
) *FiberServer {
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.GetServerReadTimeout(),
		WriteTimeout: cfg.GetServerWriteTimeout(),
		IdleTimeout:  cfg.GetServerIdleTimeout(),
		ErrorHandler: customErrorHandler,
	})

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

	s := &FiberServer{
		app:           app,
		config:        cfg,
		queryHandler:  queryHandler,
		schemaHandler: schemaHandler,
		healthHandler: healthHandler,
		bannerHandler: bannerHandler,
		logger:        logger,
	}

	s.setupRoutes()
	return s
}

// registerRoutes attaches all API routes to the given router prefix.
func (s *FiberServer) registerRoutes(r fiber.Router) {
	// Health
	r.Get("/health", s.healthHandler.HealthCheck)
	r.Get("/ready", s.healthHandler.ReadinessCheck)
	r.Get("/live", s.healthHandler.LivenessCheck)

	// Query
	query := r.Group("/query")
	query.Post("/execute", s.queryHandler.ExecuteQuery)
	query.Post("/validate", s.queryHandler.ValidateQuery)
	query.Post("/generate", s.queryHandler.GenerateQuery)

	// Schema
	schema := r.Group("/schema")
	schema.Get("/", s.schemaHandler.GetDatabaseSchema)
	schema.Get("/table/:table", s.schemaHandler.GetTableSchema)
	schema.Post("/refresh", s.schemaHandler.RefreshSchema)

	// Banner image generation
	banner := r.Group("/banner")
	banner.Post("/generate", s.bannerHandler.Generate)
	banner.Get("/generate/:id", s.bannerHandler.GetStatus)
	banner.Get("/generate/:id/variant/:index/image", s.bannerHandler.GetVariantImage)
}

func (s *FiberServer) setupRoutes() {
	// Swagger
	if s.config.Swagger.Enabled {
		s.app.Get("/swagger/*", swagger.HandlerDefault)
	}

	// 1. Root level — no prefix
	//    e.g. /banner/generate
	s.registerRoutes(s.app)

	// 2. /api/v1 — local development direct access
	//    e.g. /api/v1/banner/generate
	v1 := s.app.Group("/api/v1")
	if s.config.Swagger.Enabled {
		v1.Get("/swagger/*", swagger.HandlerDefault)
	}
	s.registerRoutes(v1)

	// 3. /public/api/v1 — gateway type: public prefix
	//    e.g. /public/api/v1/banner/generate
	publicV1 := s.app.Group("/public/api/v1")
	if s.config.Swagger.Enabled {
		publicV1.Get("/swagger/*", swagger.HandlerDefault)
	}
	s.registerRoutes(publicV1)

	// 404 catch-all
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
