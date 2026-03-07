package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func SaveToFile(path string, snap Snapshot) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	return nil
}

func LoadFromFile(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read session file: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("decode session file: %w", err)
	}
	return snap, nil
}
