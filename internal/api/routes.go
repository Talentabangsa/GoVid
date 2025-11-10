package api

import (
	"github.com/MarceloPetrucio/go-scalar-api-reference"
	"github.com/gofiber/fiber/v3"

	"govid/pkg/auth"
)

// SetupRoutes configures all API routes
func SetupRoutes(app *fiber.App, handler *Handler, validator *auth.Validator) {
	// Apply global middleware
	app.Use(LoggingMiddleware())
	app.Use(CORSMiddleware())

	// API v1 routes
	v1 := app.Group("/api/v1")

	// Health check (no auth required)
	v1.Get("/health", handler.HealthCheck)

	// Protected routes
	protected := v1.Group("")
	protected.Use(AuthMiddleware(validator))

	// Video processing endpoints
	video := protected.Group("/video")
	video.Post("/merge", handler.MergeVideos)
	video.Post("/overlay", handler.AddImageOverlay)
	video.Post("/audio", handler.AddBackgroundMusic)
	video.Post("/process", handler.ProcessComplete)

	// Job status endpoints
	jobs := protected.Group("/jobs")
	jobs.Get("/:id", handler.GetJobStatus)

	// Upload endpoints
	protected.Post("/upload", handler.UploadFile)
	protected.Post("/upload/multiple", handler.UploadMultipleFiles)

	// API documentation with Scalar (publicly accessible, no auth required)
	app.Get("/docs", func(c fiber.Ctx) error {
		htmlContent, err := scalar.ApiReferenceHTML(&scalar.Options{
			SpecURL: "/docs/swagger.yaml",
			CustomOptions: scalar.CustomOptions{
				PageTitle: "GoVid API Documentation",
			},
			DarkMode: true,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		c.Set("Content-Type", "text/html")
		return c.SendString(htmlContent)
	})

	// Serve swagger.yaml (publicly accessible, no auth required)
	app.Get("/docs/swagger.yaml", func(c fiber.Ctx) error {
		return c.SendFile("/docs/swagger.yaml")
	})
}
