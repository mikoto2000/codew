package codewconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
