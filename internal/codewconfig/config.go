package codewconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const DefaultPath = ".codew/config.json"

type Config struct {
	ShellAllow []string `json:"shell_allow"`
}

func Load(workspace string) (Config, error) {
	path := filepath.Join(workspace, DefaultPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode codew config: %w", err)
	}
	for i, raw := range cfg.ShellAllow {
		if strings.TrimSpace(raw) == "" {
			return Config{}, fmt.Errorf("shell_allow[%d] must not be empty", i)
		}
	}
	return cfg, nil
}

func AddShellAllow(workspace string, command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("shell allow command is required")
	}
	cfg, err := Load(workspace)
	if err != nil {
		return err
	}
	if !slices.Contains(cfg.ShellAllow, command) {
		cfg.ShellAllow = append(cfg.ShellAllow, command)
	}
	return Save(workspace, cfg)
}

func Save(workspace string, cfg Config) error {
	for i, raw := range cfg.ShellAllow {
		cfg.ShellAllow[i] = strings.TrimSpace(raw)
		if cfg.ShellAllow[i] == "" {
			return fmt.Errorf("shell_allow[%d] must not be empty", i)
		}
	}
	path := filepath.Join(workspace, DefaultPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode codew config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	return nil
}
