package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"govid/pkg/logger"

	"golang.org/x/sync/semaphore"
)

// Executor handles FFmpeg command execution
type Executor struct {
	binary  string
	timeout time.Duration
	sem     *semaphore.Weighted
}

// NewExecutor creates a new FFmpeg executor
func NewExecutor(binary string, timeout time.Duration, maxConcurrent int64) *Executor {
	return &Executor{
		binary:  binary,
		timeout: timeout,
		sem:     semaphore.NewWeighted(maxConcurrent),
	}
}

// Execute runs an FFmpeg command
func (e *Executor) Execute(ctx context.Context, args []string) error {
	// Acquire semaphore slot
	if err := e.sem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("failed to acquire ffmpeg slot: %w", err)
	}
	defer e.sem.Release(1)

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Build command
	cmd := exec.CommandContext(cmdCtx, e.binary, args...)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Log command
	logger.Info("Executing FFmpeg command: %s %s", e.binary, strings.Join(args, " "))

	// Execute command
	err := cmd.Run()

	// Log output
	if stdout.Len() > 0 {
		logger.Debug("FFmpeg stdout: %s", stdout.String())
	}
	if stderr.Len() > 0 {
		logger.Debug("FFmpeg stderr: %s", stderr.String())
	}

	if err != nil {
		return fmt.Errorf("ffmpeg execution failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// ValidateFile checks if a file exists
func ValidateFile(path string) error {
	if path == "" {
		return fmt.Errorf("file path is empty")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", path)
	} else if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	return nil
}

// BuildFilterComplex builds a filter_complex string
func BuildFilterComplex(filters []string) string {
	return strings.Join(filters, ";")
}

// QuoteArg quotes an argument if it contains spaces or special characters
func QuoteArg(arg string) string {
	if strings.ContainsAny(arg, " \t\n\"'") {
		return fmt.Sprintf("'%s'", strings.ReplaceAll(arg, "'", "'\\''"))
	}
	return arg
}
