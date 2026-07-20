package logging

import (
	"errors"
	"io"
	"log/slog"
)

// New constructs a JSON or text logger at one validated level.
func New(output io.Writer, level, format string) (*slog.Logger, error) {
	if output == nil {
		return nil, errors.New("log output is required")
	}
	var minimum slog.Level
	switch level {
	case "debug":
		minimum = slog.LevelDebug
	case "info":
		minimum = slog.LevelInfo
	case "warn":
		minimum = slog.LevelWarn
	case "error":
		minimum = slog.LevelError
	default:
		return nil, errors.New("unsupported log level")
	}
	options := &slog.HandlerOptions{Level: minimum}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(output, options)), nil
	}
	if format == "text" {
		return slog.New(slog.NewTextHandler(output, options)), nil
	}

	return nil, errors.New("unsupported log format")
}
