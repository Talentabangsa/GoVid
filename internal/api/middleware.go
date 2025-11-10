package api

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"govid/internal/models"
	"govid/pkg/auth"
	"govid/pkg/logger"
)

// AuthMiddleware creates a middleware for API key authentication
func AuthMiddleware(validator *auth.Validator) fiber.Handler {
	return func(c fiber.Ctx) error {
		apiKey := c.Get("X-API-Key")

		if err := validator.ValidateAPIKey(apiKey); err != nil {
			logger.Warn("Authentication failed: %v", err)
			return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
				Error:   "Unauthorized",
				Message: "Missing or invalid X-API-Key header",
			})
		}

		return c.Next()
	}
}

// LoggingMiddleware logs incoming requests
func LoggingMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		logger.Info("Request: %s %s from %s", c.Method(), c.Path(), c.IP())
		return c.Next()
	}
}

// ErrorHandlerMiddleware handles errors globally
func ErrorHandlerMiddleware(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError

	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
	}

	logger.Error("Error handling request: %v", err)

	return c.Status(code).JSON(models.ErrorResponse{
		Error:   "Internal Server Error",
		Message: err.Error(),
	})
}

// CORSMiddleware handles CORS (if needed)
func CORSMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Access-Control-Allow-Origin", "*")
		c.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")

		if c.Method() == "OPTIONS" {
			return c.SendStatus(fiber.StatusNoContent)
		}

		return c.Next()
	}
}
