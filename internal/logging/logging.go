package logging

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// New returns a zerolog logger configured for stdout.
func New() zerolog.Logger {
	return NewWithLevel("info")
}

// NewWithLevel returns a zerolog logger configured for stdout with the specified level.
// Supported levels: trace, debug, info, warn, error, fatal, panic.
// Invalid levels default to info.
func NewWithLevel(level string) zerolog.Logger {
	lvl := parseLevel(level)
	zerolog.SetGlobalLevel(lvl)
	return zerolog.New(os.Stdout).With().Timestamp().Logger().Level(lvl)
}

func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}
