// Package logging provides structured logging for AxiOS components.
package logging

import (
	"log/slog"
	"os"
)

// New creates a structured logger for the given component.
func New(component string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler).With("component", component)
}
