package tools

import "github.com/mikoto2000/codew/internal/ollama"

func Definitions() []ollama.ToolDefinition {
	return []ollama.ToolDefinition{
		{
			Type: "function",
			Function: ollama.ToolDefinitionFunc{
				Name:        "shell_exec",
				Description: "Run a shell command in the workspace.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command":     map[string]any{"type": "string"},
						"workdir":     map[string]any{"type": "string"},
						"timeout_sec": map[string]any{"type": "integer", "minimum": 1, "maximum": 300},
						"pty":         map[string]any{"type": "boolean"},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolDefinitionFunc{
				Name:        "list_files",
				Description: "List files under a path in the workspace.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":        map[string]any{"type": "string"},
						"pattern":     map[string]any{"type": "string"},
						"max_results": map[string]any{"type": "integer", "minimum": 1, "maximum": 1000},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolDefinitionFunc{
				Name:        "read_file",
				Description: "Read a UTF-8 text file from the workspace.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolDefinitionFunc{
				Name:        "write_file",
				Description: "Overwrite or create a file in the workspace.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string"},
						"content": map[string]any{"type": "string"},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolDefinitionFunc{
				Name:        "replace_in_file",
				Description: "Replace a string in a file.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":        map[string]any{"type": "string"},
						"old":         map[string]any{"type": "string"},
						"new":         map[string]any{"type": "string"},
						"replace_all": map[string]any{"type": "boolean"},
					},
					"required": []string{"path", "old", "new"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolDefinitionFunc{
				Name:        "apply_patch",
				Description: "Apply a unified diff patch safely after validation.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"patch":      map[string]any{"type": "string"},
						"check_only": map[string]any{"type": "boolean"},
					},
					"required": []string{"patch"},
				},
			},
		},
		{
			Type: "function",
			Function: ollama.ToolDefinitionFunc{
				Name:        "web_search",
				Description: "Search public web results for a query.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query":       map[string]any{"type": "string"},
						"max_results": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}
