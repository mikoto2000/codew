package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mikoto2000/codew/internal/ollama"
)

type toolBinding struct {
	server string
	name   string
}

type Manager struct {
	clients  map[string]*Client
	bindings map[string]toolBinding
	defs     []ollama.ToolDefinition
}

func NewManager() *Manager {
	return &Manager{clients: map[string]*Client{}, bindings: map[string]toolBinding{}, defs: []ollama.ToolDefinition{}}
}

func (m *Manager) LoadAndStart(ctx context.Context, workspace string, configPath string) error {
	cfgPath := configPath
	if !filepath.IsAbs(cfgPath) {
		cfgPath = filepath.Join(workspace, configPath)
	}
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, srv := range cfg.Servers {
		client := NewClient(srv.Name, srv)
		if err := client.Start(ctx, workspace); err != nil {
			return err
		}
		m.clients[srv.Name] = client
		tools, err := client.ListTools(ctx)
		if err != nil {
			return err
		}
		for _, t := range tools {
			qualified := fmt.Sprintf("mcp.%s.%s", srv.Name, t.Name)
			m.bindings[qualified] = toolBinding{server: srv.Name, name: t.Name}
			schema := t.InputSchema
			if schema == nil {
				schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			m.defs = append(m.defs, ollama.ToolDefinition{
				Type: "function",
				Function: ollama.ToolDefinitionFunc{
					Name:        qualified,
					Description: strings.TrimSpace(t.Description),
					Parameters:  schema,
				},
			})
		}
	}
	return nil
}

func (m *Manager) Close() {
	for _, c := range m.clients {
		_ = c.Close()
	}
}

func (m *Manager) Definitions() []ollama.ToolDefinition {
	out := make([]ollama.ToolDefinition, len(m.defs))
	copy(out, m.defs)
	return out
}

func (m *Manager) HasTool(name string) bool {
	_, ok := m.bindings[name]
	return ok
}

func (m *Manager) Call(ctx context.Context, qualifiedName string, rawArgs json.RawMessage) (map[string]any, error) {
	binding, ok := m.bindings[qualifiedName]
	if !ok {
		return nil, fmt.Errorf("unknown mcp tool: %s", qualifiedName)
	}
	client, ok := m.clients[binding.server]
	if !ok {
		return nil, fmt.Errorf("mcp server not found: %s", binding.server)
	}

	args := map[string]any{}
	trimmed := strings.TrimSpace(string(rawArgs))
	if trimmed != "" {
		if strings.HasPrefix(trimmed, "\"") {
			var text string
			if err := json.Unmarshal(rawArgs, &text); err != nil {
				return nil, err
			}
			if err := json.Unmarshal([]byte(text), &args); err != nil {
				return nil, err
			}
		} else {
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, err
			}
		}
	}

	result, err := client.CallTool(ctx, binding.name, args)
	if err != nil {
		return nil, err
	}
	result["mcp_server"] = binding.server
	result["mcp_tool"] = binding.name
	return result, nil
}
