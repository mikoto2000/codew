package tools

import (
	"bytes"
	"encoding/json"

	"ollama-codex-cli/internal/ollama"
)

func IsMutatingTool(name string) bool {
	switch name {
	case "write_file", "replace_in_file", "apply_patch":
		return true
	default:
		return false
	}
}

func Preview(call ollama.ToolCall) string {
	switch call.Function.Name {
	case "write_file":
		var in struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if decodeArgs(call.Function.Arguments, &in) == nil {
			preview := in.Content
			if len(preview) > 400 {
				preview = preview[:400] + "\n...<truncated>"
			}
			return "write_file path=" + in.Path + "\n" + preview
		}
	case "replace_in_file":
		var in struct {
			Path       string `json:"path"`
			Old        string `json:"old"`
			New        string `json:"new"`
			ReplaceAll bool   `json:"replace_all"`
		}
		if decodeArgs(call.Function.Arguments, &in) == nil {
			return "replace_in_file path=" + in.Path + " replace_all=" + boolString(in.ReplaceAll) + "\nold=" + trimText(in.Old, 200) + "\nnew=" + trimText(in.New, 200)
		}
	case "apply_patch":
		var in struct {
			Patch string `json:"patch"`
		}
		if decodeArgs(call.Function.Arguments, &in) == nil {
			return "apply_patch preview:\n" + trimText(in.Patch, 800)
		}
	}

	return compact(call.Function.Arguments)
}

func compact(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return string(raw)
	}
	return out.String()
}

func trimText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...<truncated>"
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
