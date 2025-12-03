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
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
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
	"govid/pkg/cleanup"
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

	// Create shutdown context
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	// Initialize shared components
	var jobWG sync.WaitGroup
	executor := ffmpeg.NewExecutor(cfg.FFmpegBinary, time.Duration(cfg.JobTimeout)*time.Second, int64(cfg.MaxConcurrentJobs))
	jobStore := models.NewJobStoreWithPersistence(cfg.JobsDir)

	// Initialize validators
	httpValidator := auth.NewValidator(cfg.HTTPAPIKey)
	mcpValidator := auth.NewValidator(cfg.MCPAPIKey)

	// Start cleanup scheduler if enabled
	var cleanupScheduler *cleanup.Scheduler
	if cfg.CleanupEnabled {
		cleanupScheduler = cleanup.NewScheduler(
			cfg.OutputDir,
			cfg.UploadDir,
			cfg.TempDir,
			jobStore,
			cfg.CleanupRetentionDays,
		)
		cleanupScheduler.Start()
		logger.Info("Cleanup scheduler enabled (retention: %d days)", cfg.CleanupRetentionDays)
	} else {
		logger.Info("Cleanup scheduler disabled")
	}

	// Start HTTP API server
	go startHTTPServer(shutdownCtx, cfg, executor, jobStore, httpValidator, &jobWG)

	// Start MCP server
	go startMCPServer(shutdownCtx, cfg, executor, jobStore, mcpValidator, &jobWG)

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down servers...")

	// Cancel shutdown context to signal servers to stop
	shutdownCancel()

	// Stop cleanup scheduler if running
	if cleanupScheduler != nil {
		cleanupScheduler.Stop()
	}

	// Wait for active jobs to finish (with timeout)
	shutdownTimeout := cfg.ShutdownTimeoutSeconds
	done := make(chan struct{})
	go func() {
		jobWG.Wait()
		close(done)
	}()

	if shutdownTimeout == 0 {
		<-done // wait forever
		logger.Info("all jobs finished")
	} else {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Duration(shutdownTimeout)*time.Second)
		defer cancel()
		select {
		case <-done:
			logger.Info("all jobs finished")
		case <-timeoutCtx.Done():
			logger.Info("shutdown timeout reached; exiting")
		}
	}
}

// startHTTPServer starts the HTTP API server
func startHTTPServer(ctx context.Context, cfg *config.Config, executor *ffmpeg.Executor, jobStore *models.JobStore, validator *auth.Validator, jobWG *sync.WaitGroup) {
	app := fiber.New(fiber.Config{
		AppName:           "GoVid API v1.0.0",
		ServerHeader:      "GoVid",
		ErrorHandler:      api.ErrorHandlerMiddleware,
		JSONEncoder:       sonic.Marshal,
		JSONDecoder:       sonic.Unmarshal,
		StreamRequestBody: true,
	})

	// Initialize handler
	handler := api.NewHandler(executor, jobStore, cfg, jobWG)

	// Setup routes
	api.SetupRoutes(app, handler, validator)

	logger.Info("HTTP API server starting on port %s", cfg.HTTPPort)

	// Shutdown goroutine
	go func() {
		<-ctx.Done()
		logger.Info("Shutting down HTTP server...")
		_ = app.ShutdownWithContext(ctx)
	}()

	if err := app.Listen(":"+cfg.HTTPPort, fiber.ListenConfig{
		DisableStartupMessage: true,
		EnablePrefork:         false,
	}); err != nil {
		logger.Error("HTTP server error: %v", err)
		os.Exit(1)
	}
}

// startMCPServer starts the MCP server
func startMCPServer(ctx context.Context, cfg *config.Config, executor *ffmpeg.Executor, jobStore *models.JobStore, validator *auth.Validator, jobWG *sync.WaitGroup) {
	// Create MCP server
	mcpServer := mcp.NewMCPServer(executor, jobStore, cfg, jobWG)

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

	// Shutdown goroutine
	go func() {
		<-ctx.Done()
		logger.Info("Shutting down MCP server...")
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("MCP server error: %v", err)
		os.Exit(1)
	}
}
