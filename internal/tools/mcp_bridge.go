package tools

import (
	"context"

	"ollama-codex-cli/internal/ollama"
)

func (e *Executor) executeMCP(call ollama.ToolCall, result map[string]any) string {
	payload, err := e.mcp.Call(context.Background(), call.Function.Name, call.Function.Arguments)
	if err != nil {
		result["ok"] = false
		result["error"] = err.Error()
		return marshalResult(result)
	}
	result["ok"] = true
	for k, v := range payload {
		result[k] = v
	}
	return marshalResult(result)
}
