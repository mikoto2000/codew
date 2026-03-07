package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ollama-codex-cli/internal/ollama"
)

const maxOutputChars = 12000

type Executor struct {
	workspace string
	profile   string
}

func NewExecutor(workspace string, profile string) (*Executor, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	return &Executor{workspace: abs, profile: NormalizeProfile(profile)}, nil
}

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
	}
}

func AllowedToolNames() map[string]struct{} {
	return AllowedToolNamesForProfile(ProfileFull)
}

func (e *Executor) Execute(call ollama.ToolCall) string {
	result := map[string]any{"tool": call.Function.Name}
	if !IsToolAllowed(e.profile, call.Function.Name) {
		result["ok"] = false
		result["error"] = fmt.Sprintf("tool %q is not allowed in profile %q", call.Function.Name, e.profile)
		data, _ := json.Marshal(result)
		return string(data)
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

	data, _ := json.Marshal(result)
	return string(data)
}

type shellArgs struct {
	Command    string `json:"command"`
	Workdir    string `json:"workdir"`
	TimeoutSec int    `json:"timeout_sec"`
	PTY        bool   `json:"pty"`
}

func (e *Executor) shellExec(raw json.RawMessage) (map[string]any, error) {
	var in shellArgs
	if err := decodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Command) == "" {
		return nil, errors.New("command is required")
	}
	if in.TimeoutSec <= 0 {
		in.TimeoutSec = 30
	}

	dir := e.workspace
	if strings.TrimSpace(in.Workdir) != "" {
		resolved, err := e.resolvePath(in.Workdir)
		if err != nil {
			return nil, err
		}
		dir = resolved
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(in.TimeoutSec)*time.Second)
	defer cancel()

	var c *exec.Cmd
	if in.PTY {
		// Use util-linux "script" for pseudo-TTY execution.
		c = exec.CommandContext(ctx, "script", "-qec", in.Command, "/dev/null")
	} else {
		c = exec.CommandContext(ctx, "bash", "-lc", in.Command)
	}
	c.Dir = dir
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()

	out := map[string]any{
		"workdir": dir,
		"command": in.Command,
		"pty":     in.PTY,
		"stdout":  truncate(stdout.String()),
		"stderr":  truncate(stderr.String()),
	}
	if err != nil {
		out["exit_error"] = err.Error()
	}
	if ctx.Err() == context.DeadlineExceeded {
		out["timed_out"] = true
	}
	return out, nil
}

type listArgs struct {
	Path       string `json:"path"`
	Pattern    string `json:"pattern"`
	MaxResults int    `json:"max_results"`
}

func (e *Executor) listFiles(raw json.RawMessage) (map[string]any, error) {
	var in listArgs
	if err := decodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 200
	}

	root := e.workspace
	if strings.TrimSpace(in.Path) != "" {
		resolved, err := e.resolvePath(in.Path)
		if err != nil {
			return nil, err
		}
		root = resolved
	}

	files := make([]string, 0, in.MaxResults)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(e.workspace, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			if rel == ".git" || strings.HasPrefix(rel, ".git/") || rel == ".gocache" || strings.HasPrefix(rel, ".gocache/") {
				return fs.SkipDir
			}
			return nil
		}
		if in.Pattern != "" {
			ok, matchErr := filepath.Match(in.Pattern, filepath.Base(path))
			if matchErr != nil || !ok {
				return nil
			}
		}
		files = append(files, rel)
		if len(files) >= in.MaxResults {
			return errors.New("limit reached")
		}
		return nil
	})
	if err != nil && err.Error() != "limit reached" {
		return nil, err
	}

	return map[string]any{
		"root":  root,
		"count": len(files),
		"files": files,
	}, nil
}

type pathArg struct {
	Path string `json:"path"`
}

func (e *Executor) readFile(raw json.RawMessage) (map[string]any, error) {
	var in pathArg
	if err := decodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Path) == "" {
		return nil, errors.New("path is required")
	}

	resolved, err := e.resolvePath(in.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, err
	}

	content := string(data)
	truncated := false
	if len(content) > maxOutputChars {
		content = content[:maxOutputChars]
		truncated = true
	}
	return map[string]any{
		"path":      in.Path,
		"content":   content,
		"truncated": truncated,
	}, nil
}

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (e *Executor) writeFile(raw json.RawMessage) (map[string]any, error) {
	var in writeArgs
	if err := decodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Path) == "" {
		return nil, errors.New("path is required")
	}

	resolved, err := e.resolvePath(in.Path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(resolved, []byte(in.Content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"path":          in.Path,
		"bytes_written": len(in.Content),
	}, nil
}

type replaceArgs struct {
	Path       string `json:"path"`
	Old        string `json:"old"`
	New        string `json:"new"`
	ReplaceAll bool   `json:"replace_all"`
}

func (e *Executor) replaceInFile(raw json.RawMessage) (map[string]any, error) {
	var in replaceArgs
	if err := decodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if in.Path == "" {
		return nil, errors.New("path is required")
	}
	if in.Old == "" {
		return nil, errors.New("old is required")
	}

	resolved, err := e.resolvePath(in.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, err
	}
	content := string(data)
	count := 1
	if in.ReplaceAll {
		count = -1
	}
	updated := strings.Replace(content, in.Old, in.New, count)
	replaced := strings.Count(content, in.Old)
	if !in.ReplaceAll && replaced > 1 {
		replaced = 1
	}
	if replaced == 0 {
		return nil, errors.New("target string not found")
	}
	if err := os.WriteFile(resolved, []byte(updated), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"path":     in.Path,
		"replaced": replaced,
	}, nil
}

type applyPatchArgs struct {
	Patch     string `json:"patch"`
	CheckOnly bool   `json:"check_only"`
}

func (e *Executor) applyPatch(raw json.RawMessage) (map[string]any, error) {
	var in applyPatchArgs
	if err := decodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Patch) == "" {
		return nil, errors.New("patch is required")
	}

	files, err := e.patchTargets(in.Patch)
	if err != nil {
		return nil, err
	}

	if _, err := runCommandWithInput("git", []string{"-C", e.workspace, "apply", "--check", "--whitespace=nowarn", "-"}, in.Patch); err != nil {
		return nil, fmt.Errorf("patch check failed: %w", err)
	}

	if in.CheckOnly {
		return map[string]any{
			"checked": true,
			"applied": false,
			"files":   files,
		}, nil
	}

	if _, err := runCommandWithInput("git", []string{"-C", e.workspace, "apply", "--whitespace=nowarn", "-"}, in.Patch); err != nil {
		return nil, fmt.Errorf("patch apply failed: %w", err)
	}

	return map[string]any{
		"checked": true,
		"applied": true,
		"files":   files,
	}, nil
}

func decodeArgs(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return errors.New("arguments are required")
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return errors.New("arguments are required")
	}

	if strings.HasPrefix(trimmed, "\"") {
		var jsonText string
		if err := json.Unmarshal(raw, &jsonText); err != nil {
			return fmt.Errorf("decode string args: %w", err)
		}
		if err := json.Unmarshal([]byte(jsonText), out); err != nil {
			return fmt.Errorf("decode nested args: %w", err)
		}
		return nil
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode args: %w", err)
	}
	return nil
}

func runCommandWithInput(bin string, args []string, stdin string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return string(out), nil
}

func (e *Executor) patchTargets(patch string) ([]string, error) {
	lines := strings.Split(patch, "\n")
	seen := map[string]struct{}{}
	out := []string{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
			path := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "--- "))
			if path == "" || path == "/dev/null" {
				continue
			}
			if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
				path = path[2:]
			}
			path = strings.Split(path, "\t")[0]
			path = strings.Trim(path, "\"")

			if _, err := e.resolvePath(path); err != nil {
				return nil, fmt.Errorf("invalid patch path %q: %w", path, err)
			}
			if _, ok := seen[path]; !ok {
				seen[path] = struct{}{}
				out = append(out, path)
			}
		}
	}
	return out, nil
}

func (e *Executor) resolvePath(path string) (string, error) {
	var joined string
	if filepath.IsAbs(path) {
		joined = filepath.Clean(path)
	} else {
		joined = filepath.Join(e.workspace, path)
	}

	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if abs == e.workspace {
		return abs, nil
	}
	prefix := e.workspace + string(os.PathSeparator)
	if !strings.HasPrefix(abs, prefix) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}
	return abs, nil
}

func truncate(s string) string {
	if len(s) <= maxOutputChars {
		return s
	}
	return s[:maxOutputChars] + "\n...<truncated>"
}
