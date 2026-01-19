package state

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

// FileStore persists state as JSON on disk.
type FileStore struct {
	path   string
	logger zerolog.Logger
}

// NewFileStore returns a JSON-backed state store.
func NewFileStore(path string, logger zerolog.Logger) *FileStore {
	return &FileStore{
		path:   path,
		logger: logger,
	}
}

// Load reads state from disk. Missing or corrupt files return an empty state with a warning.
func (s *FileStore) Load(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.logger.Warn().Str("path", s.path).Msg("state file missing, starting fresh")
			return State{Version: CurrentStateVersion, Stacks: map[string]StackSnapshot{}}, nil
		}
		return State{}, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		s.logger.Warn().Str("path", s.path).Err(err).Msg("state file corrupt, starting fresh")
		return State{Version: CurrentStateVersion, Stacks: map[string]StackSnapshot{}}, nil
	}

	// Handle version migration
	if state.Version == 0 {
		// Pre-versioned state file, upgrade to current version
		state.Version = CurrentStateVersion
		s.logger.Info().Str("path", s.path).Msg("migrated state file to version 1")
	}
	if state.Version > CurrentStateVersion {
		s.logger.Warn().
			Str("path", s.path).
			Int("file_version", state.Version).
			Int("supported_version", CurrentStateVersion).
			Msg("state file version newer than supported, starting fresh")
		return State{Version: CurrentStateVersion, Stacks: map[string]StackSnapshot{}}, nil
	}

	if state.Stacks == nil {
		state.Stacks = map[string]StackSnapshot{}
	}
	return state, nil
}

// Save writes state to disk atomically.
func (s *FileStore) Save(ctx context.Context, state State) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Ensure version is set
	if state.Version == 0 {
		state.Version = CurrentStateVersion
	}
	if state.Stacks == nil {
		state.Stacks = map[string]StackSnapshot{}
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Create temp file with restrictive permissions (0600) to protect state data
	tempFile, err := os.CreateTemp(dir, ".state-*.json")
	if err != nil {
		return err
	}

	// Ensure restrictive permissions on temp file before writing data
	if err := os.Chmod(tempFile.Name(), 0o600); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return err
	}

	cleanup := func() {
		_ = os.Remove(tempFile.Name())
	}

	encoder := json.NewEncoder(tempFile)
	if err := encoder.Encode(state); err != nil {
		_ = tempFile.Close()
		cleanup()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		cleanup()
		return err
	}
	if err := tempFile.Close(); err != nil {
		cleanup()
		return err
	}

	if err := os.Rename(tempFile.Name(), s.path); err != nil {
		cleanup()
		return err
	}

	if dirHandle, err := os.Open(dir); err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}

	return nil
}
