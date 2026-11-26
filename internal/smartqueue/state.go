package smartqueue

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// State is the smart queue file: the switch, the merging snapshot, and prioritize ids.
type State struct {
	Enabled            bool     `json:"enabled"`
	HasMergingSnapshot bool     `json:"hasMergingSnapshot"`
	AutomaticMerging   bool     `json:"automaticMerging"`
	PrioritizedJobIDs  []string `json:"prioritizedJobIds"`
}

// LoadState reads the state file at path.
// A missing file is off: enabled false, no merging snapshot, and no prioritize ids.
func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the configured state file, not request input
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("read state file: %w", err)
	}
	var state State
	if unmarshalErr := json.Unmarshal(data, &state); unmarshalErr != nil {
		return State{}, fmt.Errorf("decode state file: %w", unmarshalErr)
	}
	return state, nil
}

// SaveState writes path atomically, creating the parent directory first.
// If that directory cannot be created, the error is returned and no state file is left behind.
func SaveState(path string, state State) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode state file: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".smart-queue-*.tmp")
	if err != nil {
		return fmt.Errorf("create state temp file: %w", err)
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()

	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()
		return fmt.Errorf("write state temp file: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		return fmt.Errorf("close state temp file: %w", closeErr)
	}
	if renameErr := os.Rename(tmpName, path); renameErr != nil {
		return fmt.Errorf("replace state file: %w", renameErr)
	}
	renamed = true
	return nil
}
