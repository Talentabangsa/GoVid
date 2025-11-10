// Package main is the entry point for the GoVid application
// @title GoVid API
// @version 1.0
// @description FFmpeg video processing API with MCP server support

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description API key for authentication
package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/mark3labs/mcp-go/server"

	"govid/internal/api"
	"govid/internal/ffmpeg"
	"govid/internal/mcp"
	"govid/internal/models"
	"govid/pkg/auth"
	"govid/pkg/config"
	"govid/pkg/logger"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("Failed to load configuration: %v", err)
		os.Exit(1)
	}

	logger.Info("Starting GoVid application...")
	logger.Info("HTTP API Port: %s", cfg.HTTPPort)
	logger.Info("MCP Server Port: %s", cfg.MCPPort)

	// Initialize shared components
	executor := ffmpeg.NewExecutor(cfg.FFmpegBinary, time.Duration(cfg.JobTimeout)*time.Second)
	jobStore := models.NewJobStore()

	// Initialize validators
	httpValidator := auth.NewValidator(cfg.HTTPAPIKey)
	mcpValidator := auth.NewValidator(cfg.MCPAPIKey)

	// Start HTTP API server
	go startHTTPServer(cfg, executor, jobStore, httpValidator)

	// Start MCP server
	go startMCPServer(cfg, executor, jobStore, mcpValidator)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down servers...")
}

// startHTTPServer starts the HTTP API server
func startHTTPServer(cfg *config.Config, executor *ffmpeg.Executor, jobStore *models.JobStore, validator *auth.Validator) {
	app := fiber.New(fiber.Config{
		AppName:           "GoVid API v1.0.0",
		ServerHeader:      "GoVid",
		ErrorHandler:      api.ErrorHandlerMiddleware,
		JSONEncoder:       sonic.Marshal,
		JSONDecoder:       sonic.Unmarshal,
		StreamRequestBody: true,
	})

	// Initialize handler
	handler := api.NewHandler(executor, jobStore, cfg)

	// Setup routes
	api.SetupRoutes(app, handler, validator)

	logger.Info("HTTP API server starting on port %s", cfg.HTTPPort)

	if err := app.Listen(":"+cfg.HTTPPort, fiber.ListenConfig{
		DisableStartupMessage: true,
		EnablePrefork:         false,
	}); err != nil {
		logger.Error("HTTP server error: %v", err)
		os.Exit(1)
	}
}

// startMCPServer starts the MCP server
func startMCPServer(cfg *config.Config, executor *ffmpeg.Executor, jobStore *models.JobStore, validator *auth.Validator) {
	// Create MCP server
	mcpServer := mcp.NewMCPServer(executor, jobStore, cfg)

	// Create StreamableHTTP server
	httpServer := server.NewStreamableHTTPServer(
		mcpServer.GetServer(),
		server.WithEndpointPath("/mcp"),
	)

	// Create HTTP mux with middleware
	mux := http.NewServeMux()

	// Wrap MCP handler with auth middleware
	mcpHandler := mcp.AuthMiddleware(validator)(httpServer)
	mcpHandler = mcp.LoggingMiddleware(mcpHandler)
	mcpHandler = mcp.CORSMiddleware(mcpHandler)

	mux.Handle("/mcp", mcpHandler)

	// Add root path handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"name":"GoVid MCP Server","version":"1.0.0","endpoints":{"/mcp":"MCP StreamableHTTP endpoint","/health":"Health check"}}`)
			return
		}
		http.NotFound(w, r)
	})

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","server":"mcp"}`)
	})

	logger.Info("MCP server starting on port %s", cfg.MCPPort)

	srv := &http.Server{
		Addr:    ":" + cfg.MCPPort,
		Handler: mux,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("MCP server error: %v", err)
		os.Exit(1)
	}
}
