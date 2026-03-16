package tools

import (
	"encoding/json"
	"fmt"

	"github.com/mikoto2000/codew/internal/ollama"
)

func (e *Executor) preflight(call ollama.ToolCall, sandboxMode string) (map[string]any, bool) {
	result := map[string]any{"tool": call.Function.Name}
	if e.mcp != nil && e.mcp.HasTool(call.Function.Name) {
		if err := CheckPermissions(sandboxMode, RequiredPermissions(call.Function.Name, true)); err != nil {
			result["ok"] = false
			result["error"] = err.Error()
			return result, false
		}
		if e.profile != ProfileFull {
			result["ok"] = false
			result["error"] = fmt.Sprintf("mcp tool %q is allowed only in profile %q", call.Function.Name, ProfileFull)
			return result, false
		}
		return result, true
	}
	if err := CheckPermissions(sandboxMode, RequiredPermissions(call.Function.Name, false)); err != nil {
		result["ok"] = false
		result["error"] = err.Error()
		return result, false
	}
	if !IsToolAllowed(e.profile, call.Function.Name) {
		result["ok"] = false
		result["error"] = fmt.Sprintf("tool %q is not allowed in profile %q", call.Function.Name, e.profile)
		return result, false
	}
	return result, true
}

func marshalResult(result map[string]any) string {
	data, _ := json.Marshal(result)
	return string(data)
}
