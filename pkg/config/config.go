package config

import (
	"fmt"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
)

// Config holds all application configuration
type Config struct {
	// Server configuration
	HTTPPort string `env:"HTTP_PORT" env-default:"4101"`
	MCPPort  string `env:"MCP_PORT" env-default:"1106"`

	// Authentication
	HTTPAPIKey string `env:"HTTP_API_KEY" env-required:"true"`
	MCPAPIKey  string `env:"MCP_API_KEY" env-required:"true"`

	// FFmpeg configuration
	FFmpegBinary string `env:"FFMPEG_BINARY" env-default:"ffmpeg"`

	// File storage
	UploadDir string `env:"UPLOAD_DIR" env-default:"./uploads"`
	OutputDir string `env:"OUTPUT_DIR" env-default:"./outputs"`
	TempDir   string `env:"TEMP_DIR" env-default:"./temp"`
	JobsDir   string `env:"JOBS_DIR" env-default:"./jobs"`

	// Job configuration
	MaxConcurrentJobs int `env:"MAX_CONCURRENT_JOBS" env-default:"3"`
	JobTimeout        int `env:"JOB_TIMEOUT" env-default:"3600"` // in seconds
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	var cfg Config

	// Read configuration from environment
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Create necessary directories
	dirs := []string{cfg.UploadDir, cfg.OutputDir, cfg.TempDir, cfg.JobsDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return &cfg, nil
}
