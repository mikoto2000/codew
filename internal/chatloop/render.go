package chatloop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/peterh/liner"

	"github.com/mikoto2000/codew/internal/ollama"
)

func AskToolApproval(lineEditor *liner.State, call ollama.ToolCall, preview string) (approved bool, allowAll bool) {
	args := CompactJSON(call.Function.Arguments)
	if preview != "" {
		fmt.Printf("\n[tool:%s preview]\n%s\n", call.Function.Name, ColorizeDiff(preview))
	}
	line, err := lineEditor.Prompt(fmt.Sprintf("approve tool %s args=%s ? [y/N/a]: ", call.Function.Name, args))
	if err != nil {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, false
	case "a", "all":
		return true, true
	default:
		return false, false
	}
}

func AskNetworkEscalation(lineEditor *liner.State, toolName string) (allowOnce bool, allowAlways bool) {
	line, err := lineEditor.Prompt(fmt.Sprintf("network escalation required for %s. allow once [y], always [a], deny [N]: ", toolName))
	if err != nil {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, false
	case "a", "all", "always":
		return false, true
	default:
		return false, false
	}
}

func AskShellAllowlist(lineEditor *liner.State, command string) (allowOnce bool, allowAlways bool) {
	line, err := lineEditor.Prompt(fmt.Sprintf("shell command %q is not in allowlist. allow once [y], add to config [a], deny [N]: ", command))
	if err != nil {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, false
	case "a", "all", "always":
		return false, true
	default:
		return false, false
	}
}

func CompactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return string(raw)
	}
	return out.String()
}

func SummarizeToolResult(raw string) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "done"
	}
	ok := asBool(obj["ok"])
	tool := asString(obj["tool"])
	if !ok {
		errMsg := asString(obj["error"])
		if errMsg == "" {
			errMsg = "failed"
		}
		return "ok=false error=" + errMsg
	}

	switch tool {
	case "shell_exec":
		return fmt.Sprintf("ok=true exit_error=%q timed_out=%t", asString(obj["exit_error"]), asBool(obj["timed_out"]))
	case "replace_in_file":
		return fmt.Sprintf("ok=true replaced=%d path=%s", asInt(obj["replaced"]), asString(obj["path"]))
	case "write_file":
		return fmt.Sprintf("ok=true bytes_written=%d path=%s", asInt(obj["bytes_written"]), asString(obj["path"]))
	case "apply_patch":
		return fmt.Sprintf("ok=true checked=%t applied=%t", asBool(obj["checked"]), asBool(obj["applied"]))
	case "list_files":
		return fmt.Sprintf("ok=true files=%d", asInt(obj["count"]))
	case "post_validate":
		return fmt.Sprintf("ok=%t commands=%d", ok, lenAny(obj["commands"]))
	default:
		return "ok=true"
	}
}

func ColorizeDiff(text string) string {
	var out strings.Builder
	for _, line := range strings.Split(text, "\n") {
		color := ""
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			color = "\033[36m"
		case strings.HasPrefix(line, "@@"):
			color = "\033[33m"
		case strings.HasPrefix(line, "+"):
			color = "\033[32m"
		case strings.HasPrefix(line, "-"):
			color = "\033[31m"
		}
		if color != "" {
			out.WriteString(color)
			out.WriteString(line)
			out.WriteString("\033[0m")
		} else {
			out.WriteString(line)
		}
		out.WriteByte('\n')
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func lenAny(v any) int {
	switch arr := v.(type) {
	case []any:
		return len(arr)
	default:
		return 0
	}
}
