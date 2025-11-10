package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

var logger zerolog.Logger

func init() {
	// Configure zerolog
	zerolog.TimeFieldFormat = time.RFC3339

	// Use console writer for pretty output
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05",
	}

	logger = zerolog.New(output).With().Timestamp().Logger()
}

// Info logs an info level message
func Info(format string, v ...any) {
	logger.Info().Msgf(format, v...)
}

// Error logs an error level message
func Error(format string, v ...any) {
	logger.Error().Msgf(format, v...)
}

// Warn logs a warning level message
func Warn(format string, v ...any) {
	logger.Warn().Msgf(format, v...)
}

// Debug logs a debug level message
func Debug(format string, v ...any) {
	logger.Debug().Msgf(format, v...)
}

// Fatal logs a fatal level message and exits
func Fatal(format string, v ...any) {
	logger.Fatal().Msgf(format, v...)
}
