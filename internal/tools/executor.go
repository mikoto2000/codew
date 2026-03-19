package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mikoto2000/codew/internal/mcp"
	"github.com/mikoto2000/codew/internal/ollama"
)

const maxOutputChars = 12000

type Executor struct {
	workspace  string
	profile    string
	dryRun     bool
	mcp        *mcp.Manager
	sandbox    string
	shellAllow []string
}

func NewExecutor(workspace string, profile string, dryRun bool, sandboxMode string, mcpManager *mcp.Manager, shellAllow []string) (*Executor, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	return &Executor{
		workspace:  abs,
		profile:    NormalizeProfile(profile),
		dryRun:     dryRun,
		mcp:        mcpManager,
		sandbox:    NormalizeSandboxMode(sandboxMode),
		shellAllow: append([]string(nil), shellAllow...),
	}, nil
}

func AllowedToolNames() map[string]struct{} {
	return AllowedToolNamesForProfile(ProfileFull)
}

func (e *Executor) Execute(call ollama.ToolCall) string {
	return e.executeWithSandbox(call, e.sandbox)
}

func (e *Executor) ExecuteWithSandbox(call ollama.ToolCall, sandboxMode string) string {
	return e.executeWithSandbox(call, NormalizeSandboxMode(sandboxMode))
}

func (e *Executor) executeWithSandbox(call ollama.ToolCall, sandboxMode string) string {
	result, ok := e.preflight(call, sandboxMode)
	if !ok {
		return marshalResult(result)
	}
	if e.mcp != nil && e.mcp.HasTool(call.Function.Name) {
		return e.executeMCP(call, result)
	}
	if e.dryRun && IsMutatingTool(call.Function.Name) {
		plan, err := e.dryRunPlan(call)
		if err != nil {
			result["ok"] = false
			result["error"] = err.Error()
		} else {
			result["ok"] = true
			result["dry_run"] = true
			for k, v := range plan {
				result[k] = v
			}
		}
		return marshalResult(result)
	}

	var (
		payload map[string]any
		err     error
	)
	switch call.Function.Name {
	case "shell_exec":
		payload, err = e.shellExec(call.Function.Arguments)
	case "list_files":
		payload, err = e.listFiles(call.Function.Arguments)
	case "read_file":
		payload, err = e.readFile(call.Function.Arguments)
	case "write_file":
		payload, err = e.writeFile(call.Function.Arguments)
	case "replace_in_file":
		payload, err = e.replaceInFile(call.Function.Arguments)
	case "apply_patch":
		payload, err = e.applyPatch(call.Function.Arguments)
	case "web_search":
		payload, err = e.webSearch(call.Function.Arguments)
	default:
		err = fmt.Errorf("unknown tool: %s", call.Function.Name)
	}

	if err != nil {
		result["ok"] = false
		result["error"] = err.Error()
	} else {
		result["ok"] = true
		for k, v := range payload {
			result[k] = v
		}
	}

	return marshalResult(result)
}

func (e *Executor) dryRunPlan(call ollama.ToolCall) (map[string]any, error) {
	switch call.Function.Name {
	case "write_file":
		var in writeArgs
		if err := decodeArgs(call.Function.Arguments, &in); err != nil {
			return nil, err
		}
		return map[string]any{
			"plan":          "write_file",
			"path":          in.Path,
			"bytes_written": len(in.Content),
		}, nil
	case "replace_in_file":
		var in replaceArgs
		if err := decodeArgs(call.Function.Arguments, &in); err != nil {
			return nil, err
		}
		return map[string]any{
			"plan":        "replace_in_file",
			"path":        in.Path,
			"old_preview": trimForPlan(in.Old),
			"new_preview": trimForPlan(in.New),
			"replace_all": in.ReplaceAll,
		}, nil
	case "apply_patch":
		var in applyPatchArgs
		if err := decodeArgs(call.Function.Arguments, &in); err != nil {
			return nil, err
		}
		files, err := e.patchTargets(in.Patch)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"plan":    "apply_patch",
			"files":   files,
			"preview": trimForPlan(in.Patch),
		}, nil
	default:
		return nil, fmt.Errorf("dry-run not supported for tool: %s", call.Function.Name)
	}
}

func trimForPlan(s string) string {
	if len(s) <= 400 {
		return s
	}
	return s[:400] + "\n...<truncated>"
}

func decodeArgs(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func runCommandWithInput(bin string, args []string, stdin string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (e *Executor) patchTargets(patch string) ([]string, error) {
	lines := strings.Split(patch, "\n")
	files := []string{}
	seen := map[string]struct{}{}
	for _, line := range lines {
		if !strings.HasPrefix(line, "+++ b/") {
			continue
		}
		path := strings.TrimPrefix(line, "+++ b/")
		if path == "" || path == "/dev/null" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("patch does not contain file targets")
	}
	return files, nil
}

func splitPatchByFile(patch string) []string {
	lines := strings.Split(patch, "\n")
	chunks := []string{}
	var current []string
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") && len(current) > 0 {
			chunks = append(chunks, strings.Join(current, "\n")+"\n")
			current = nil
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, "\n"))
	}
	return chunks
}

func (e *Executor) resolvePath(path string) (string, error) {
	clean := filepath.Clean(path)
	abs := filepath.Join(e.workspace, clean)
	resolved, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(e.workspace, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}
	return resolved, nil
}

func truncate(s string) string {
	if len(s) <= maxOutputChars {
		return s
	}
	return s[:maxOutputChars] + "\n...<truncated>"
}
