package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Config struct {
	Servers []ServerConfig `json:"servers"`
}

type ServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Cwd     string            `json:"cwd"`
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode mcp config: %w", err)
	}
	for _, s := range cfg.Servers {
		if s.Name == "" {
			return Config{}, errors.New("mcp server name is required")
		}
		if s.Command == "" {
			return Config{}, fmt.Errorf("mcp server %q command is required", s.Name)
		}
	}
	return cfg, nil
}
