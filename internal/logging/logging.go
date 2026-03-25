// Package logging provides a thin slog initialization layer for MDEMG.
// Call Init() once at startup to configure the global slog default and bridge
// stdlib log output through slog.
package logging

import (
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
)

// Init configures the global slog logger and bridges stdlib log through it.
// format is "text" or "json". level is "debug", "info", "warn", or "error".
// w is the output writer (nil defaults to os.Stderr).
func Init(format, level string, w io.Writer) {
	if w == nil {
		w = os.Stderr
	}

	lvl := ParseLevel(level)

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: lvl}
	if strings.EqualFold(format, "json") {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Bridge stdlib log.Printf/Println through slog so third-party and
	// not-yet-migrated callers route through the structured pipeline.
	log.SetFlags(0)
	log.SetOutput(slog.NewLogLogger(logger.Handler(), lvl).Writer())
}

// ParseLevel converts a level string to slog.Level.
// Accepts: "debug", "info", "warn", "error" (case-insensitive).
// Defaults to slog.LevelInfo for unrecognized values.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
