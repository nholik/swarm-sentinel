package logging

import (
	"os"

	"github.com/rs/zerolog"
)

// New returns a zerolog logger configured for stdout.
func New() zerolog.Logger {
	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}
