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
// Invalid levels default to info with a warning.
func NewWithLevel(level string) zerolog.Logger {
	lvl, valid := parseLevel(level)
	zerolog.SetGlobalLevel(lvl)
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger().Level(lvl)
	if !valid && level != "" {
		logger.Warn().Str("provided", level).Str("using", "info").Msg("unrecognized log level, defaulting to info")
	}
	return logger
}

func parseLevel(level string) (zerolog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		return zerolog.TraceLevel, true
	case "debug":
		return zerolog.DebugLevel, true
	case "info", "":
		return zerolog.InfoLevel, true
	case "warn", "warning":
		return zerolog.WarnLevel, true
	case "error":
		return zerolog.ErrorLevel, true
	case "fatal":
		return zerolog.FatalLevel, true
	case "panic":
		return zerolog.PanicLevel, true
	default:
		return zerolog.InfoLevel, false
	}
}
