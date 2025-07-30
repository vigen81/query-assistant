package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"gitlab.smartbet.am/golang/query-assistant/internal/config"
)

// AuthMiddleware validates JWT tokens
func AuthMiddleware(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip auth if configured
		if cfg.Auth.SkipAuth {
			return c.Next()
		}

		// Get token from header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":     "Missing authorization header",
				"code":      "MISSING_AUTH",
				"timestamp": time.Now(),
			})
		}

		// Extract token
		tokenString := strings.Replace(authHeader, "Bearer ", "", 1)

		// Parse and validate token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.ErrUnauthorized
			}

			// Return secret key from config
			return []byte(cfg.Auth.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":     "Invalid token",
				"code":      "INVALID_TOKEN",
				"timestamp": time.Now(),
			})
		}

		// Extract user information from claims
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if userID, exists := claims["user_id"]; exists {
				c.Locals("user_id", userID)
			}
			if username, exists := claims["username"]; exists {
				c.Locals("username", username)
			}
			if role, exists := claims["role"]; exists {
				c.Locals("user_role", role)
			}
		}

		return c.Next()
	}
}

// RateLimitMiddleware provides rate limiting
func RateLimitMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// TODO: Implement rate limiting
		// For now, just pass through
		return c.Next()
	}
}

// LoggingMiddleware adds structured logging
func LoggingMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Add request-specific fields to context
		c.Locals("start_time", time.Now())

		err := c.Next()

		// Log request details after processing
		// This could be enhanced with more detailed logging

		return err
	}
}
