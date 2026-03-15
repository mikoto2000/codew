package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"ollama-codex-cli/internal/ollama"
)

type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ToolParseResult struct {
	Calls       []ollama.ToolCall `json:"calls"`
	Parsed      bool              `json:"parsed"`
	Diagnostics []Diagnostic      `json:"diagnostics,omitempty"`
}

type simpleToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type wrappedToolCall struct {
	ToolCall  *simpleToolCall  `json:"tool_call"`
	ToolCalls []simpleToolCall `json:"tool_calls"`
}

func ExtractToolCalls(msg ollama.Message, allowed map[string]struct{}) ToolParseResult {
	if len(msg.ToolCalls) > 0 {
		return ToolParseResult{
			Calls:  filterAllowed(msg.ToolCalls, allowed),
			Parsed: true,
		}
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return ToolParseResult{}
	}

	candidates := []string{content}
	if block := extractJSONCodeBlock(content); block != "" && block != content {
		candidates = append(candidates, block)
	}

	combined := ToolParseResult{}
	for _, candidate := range candidates {
		result := parseCandidate(candidate, allowed)
		if result.Parsed && len(result.Calls) > 0 {
			return result
		}
		combined.Parsed = combined.Parsed || result.Parsed
		combined.Diagnostics = append(combined.Diagnostics, result.Diagnostics...)
	}
	combined.Diagnostics = dedupeDiagnostics(combined.Diagnostics)
	return combined
}

func FormatDiagnostics(diags []Diagnostic) string {
	if len(diags) == 0 {
		return ""
	}
	parts := make([]string, 0, len(diags))
	for _, diag := range diags {
		parts = append(parts, fmt.Sprintf("%s: %s", diag.Code, diag.Message))
	}
	return strings.Join(parts, "; ")
}

func parseCandidate(candidate string, allowed map[string]struct{}) ToolParseResult {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ToolParseResult{}
	}

	var direct simpleToolCall
	if err := json.Unmarshal([]byte(candidate), &direct); err == nil && direct.Name != "" {
		call, diags, ok := toToolCall(direct, allowed)
		return ToolParseResult{
			Calls:       singleCall(call, ok),
			Parsed:      true,
			Diagnostics: diags,
		}
	}

	var wrapped wrappedToolCall
	if err := json.Unmarshal([]byte(candidate), &wrapped); err == nil {
		result := ToolParseResult{Parsed: wrapped.ToolCall != nil || len(wrapped.ToolCalls) > 0}
		if wrapped.ToolCall != nil {
			call, diags, ok := toToolCall(*wrapped.ToolCall, allowed)
			result.Diagnostics = append(result.Diagnostics, diags...)
			if ok {
				result.Calls = append(result.Calls, call)
			}
		}
		if len(wrapped.ToolCalls) > 0 {
			for _, c := range wrapped.ToolCalls {
				call, diags, ok := toToolCall(c, allowed)
				result.Diagnostics = append(result.Diagnostics, diags...)
				if ok {
					result.Calls = append(result.Calls, call)
				}
			}
		}
		result.Diagnostics = dedupeDiagnostics(result.Diagnostics)
		return result
	}

	var arr []simpleToolCall
	if err := json.Unmarshal([]byte(candidate), &arr); err == nil && len(arr) > 0 {
		result := ToolParseResult{Parsed: true}
		for _, c := range arr {
			call, diags, ok := toToolCall(c, allowed)
			result.Diagnostics = append(result.Diagnostics, diags...)
			if ok {
				result.Calls = append(result.Calls, call)
			}
		}
		result.Diagnostics = dedupeDiagnostics(result.Diagnostics)
		return result
	}

	if looksLikeJSON(candidate) {
		return ToolParseResult{
			Parsed: true,
			Diagnostics: []Diagnostic{{
				Code:    "malformed_json",
				Message: "tool call JSON could not be parsed",
			}},
		}
	}

	return ToolParseResult{}
}

func toToolCall(in simpleToolCall, allowed map[string]struct{}) (ollama.ToolCall, []Diagnostic, bool) {
	if _, ok := allowed[in.Name]; !ok {
		return ollama.ToolCall{}, []Diagnostic{{
			Code:    "rejected_by_allowlist",
			Message: fmt.Sprintf("tool %q is not allowed", in.Name),
		}}, false
	}
	args := in.Arguments
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if !isJSONObject(args) {
		return ollama.ToolCall{}, []Diagnostic{{
			Code:    "invalid_arguments",
			Message: fmt.Sprintf("tool %q arguments must be a JSON object", in.Name),
		}}, false
	}
	return ollama.ToolCall{
		Type: "function",
		Function: ollama.ToolFunctionCall{
			Name:      in.Name,
			Arguments: args,
		},
	}, nil, true
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

func looksLikeJSON(candidate string) bool {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return false
	}
	switch trimmed[0] {
	case '{', '[':
		return true
	default:
		return false
	}
}

func isJSONObject(raw json.RawMessage) bool {
	var obj map[string]any
	return json.Unmarshal(raw, &obj) == nil
}

func singleCall(call ollama.ToolCall, ok bool) []ollama.ToolCall {
	if !ok {
		return nil
	}
	return []ollama.ToolCall{call}
}

func dedupeDiagnostics(diags []Diagnostic) []Diagnostic {
	if len(diags) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]Diagnostic, 0, len(diags))
	for _, diag := range diags {
		key := diag.Code + "\x00" + diag.Message
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, diag)
	}
	return out
}
