package agent

import (
	"encoding/json"
	"strings"

	"ollama-codex-cli/internal/ollama"
)

type simpleToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type wrappedToolCall struct {
	ToolCall  *simpleToolCall  `json:"tool_call"`
	ToolCalls []simpleToolCall `json:"tool_calls"`
}

func ExtractToolCalls(msg ollama.Message, allowed map[string]struct{}) ([]ollama.ToolCall, bool) {
	if len(msg.ToolCalls) > 0 {
		return filterAllowed(msg.ToolCalls, allowed), true
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil, false
	}

	candidates := []string{content}
	if block := extractJSONCodeBlock(content); block != "" {
		candidates = append(candidates, block)
	}

	for _, candidate := range candidates {
		calls, ok := parseCandidate(candidate, allowed)
		if ok && len(calls) > 0 {
			return calls, true
		}
	}
	return nil, false
}

func parseCandidate(candidate string, allowed map[string]struct{}) ([]ollama.ToolCall, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return nil, false
	}

	var direct simpleToolCall
	if err := json.Unmarshal([]byte(candidate), &direct); err == nil && direct.Name != "" {
		call, ok := toToolCall(direct, allowed)
		if ok {
			return []ollama.ToolCall{call}, true
		}
	}

	var wrapped wrappedToolCall
	if err := json.Unmarshal([]byte(candidate), &wrapped); err == nil {
		if wrapped.ToolCall != nil {
			call, ok := toToolCall(*wrapped.ToolCall, allowed)
			if ok {
				return []ollama.ToolCall{call}, true
			}
		}
		if len(wrapped.ToolCalls) > 0 {
			out := make([]ollama.ToolCall, 0, len(wrapped.ToolCalls))
			for _, c := range wrapped.ToolCalls {
				if call, ok := toToolCall(c, allowed); ok {
					out = append(out, call)
				}
			}
			if len(out) > 0 {
				return out, true
			}
		}
	}

	var arr []simpleToolCall
	if err := json.Unmarshal([]byte(candidate), &arr); err == nil && len(arr) > 0 {
		out := make([]ollama.ToolCall, 0, len(arr))
		for _, c := range arr {
			if call, ok := toToolCall(c, allowed); ok {
				out = append(out, call)
			}
		}
		if len(out) > 0 {
			return out, true
		}
	}

	return nil, false
}

func toToolCall(in simpleToolCall, allowed map[string]struct{}) (ollama.ToolCall, bool) {
	if _, ok := allowed[in.Name]; !ok {
		return ollama.ToolCall{}, false
	}
	args := in.Arguments
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	return ollama.ToolCall{
		Type: "function",
		Function: ollama.ToolFunctionCall{
			Name:      in.Name,
			Arguments: args,
		},
	}, true
}

func filterAllowed(calls []ollama.ToolCall, allowed map[string]struct{}) []ollama.ToolCall {
	out := make([]ollama.ToolCall, 0, len(calls))
	for _, call := range calls {
		if _, ok := allowed[call.Function.Name]; ok {
			out = append(out, call)
		}
	}
	return out
}

func extractJSONCodeBlock(text string) string {
	start := strings.Index(text, "```json")
	if start == -1 {
		start = strings.Index(text, "```")
		if start == -1 {
			return ""
		}
	}
	start = strings.Index(text[start:], "\n")
	if start == -1 {
		return ""
	}

	first := strings.Index(text, "```")
	if first == -1 {
		return ""
	}
	rest := text[first+3:]
	if strings.HasPrefix(rest, "json") {
		rest = strings.TrimPrefix(rest, "json")
	}
	rest = strings.TrimPrefix(rest, "\n")
	end := strings.Index(rest, "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}
