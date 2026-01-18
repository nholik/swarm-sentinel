package main

import (
	"github.com/nholik/swarm-sentinel/internal/logging"
)

func main() {
	logger := logging.New()
	logger.Info().Msg("swarm-sentinel starting")
}
