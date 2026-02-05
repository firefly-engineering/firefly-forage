package logging

import (
	"io"
	"log/slog"
	"os"
)

var (
	// Logger is the global structured logger
	Logger *slog.Logger

	// Verbose enables debug logging
	Verbose bool
)

func init() {
	// Default to a simple text handler for CLI output
	Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// Setup configures the logger based on verbosity and output preferences
func Setup(verbose bool, jsonOutput bool, w io.Writer) {
	Verbose = verbose

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	if w == nil {
		w = os.Stderr
	}

	if jsonOutput {
		Logger = slog.New(slog.NewJSONHandler(w, opts))
	} else {
		Logger = slog.New(slog.NewTextHandler(w, opts))
	}
}

// Debug logs a debug message
func Debug(msg string, args ...any) {
	Logger.Debug(msg, args...)
}

// Info logs an info message
func Info(msg string, args ...any) {
	Logger.Info(msg, args...)
}

// Warn logs a warning message
func Warn(msg string, args ...any) {
	Logger.Warn(msg, args...)
}

// Error logs an error message
func Error(msg string, args ...any) {
	Logger.Error(msg, args...)
}

// With returns a logger with additional attributes
func With(args ...any) *slog.Logger {
	return Logger.With(args...)
}
