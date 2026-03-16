package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

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
		if _, err3 := runCommandWithInput("git", []string{"-C", e.workspace, "apply", "--check", "--3way", "--whitespace=nowarn", "-"}, in.Patch); err3 != nil {
			chunks := splitPatchByFile(in.Patch)
			if len(chunks) <= 1 {
				return nil, fmt.Errorf("patch check failed: %w", err)
			}
			failed := []string{}
			for _, chunk := range chunks {
				if _, chunkErr := runCommandWithInput("git", []string{"-C", e.workspace, "apply", "--check", "--whitespace=nowarn", "-"}, chunk); chunkErr != nil {
					failed = append(failed, chunkErr.Error())
				}
			}
			if len(failed) > 0 {
				return nil, fmt.Errorf("patch check failed (chunked): %s", strings.Join(failed, " | "))
			}
		}
	}

	if in.CheckOnly {
		return map[string]any{
			"checked": true,
			"applied": false,
			"files":   files,
		}, nil
	}

	if _, err := runCommandWithInput("git", []string{"-C", e.workspace, "apply", "--whitespace=nowarn", "-"}, in.Patch); err != nil {
		if _, err3 := runCommandWithInput("git", []string{"-C", e.workspace, "apply", "--3way", "--whitespace=nowarn", "-"}, in.Patch); err3 == nil {
			return map[string]any{
				"checked":   true,
				"applied":   true,
				"files":     files,
				"fallback":  "3way",
				"recovered": true,
			}, nil
		}
		chunks := splitPatchByFile(in.Patch)
		if len(chunks) > 1 {
			applied := 0
			failed := []string{}
			for _, chunk := range chunks {
				if _, chunkErr := runCommandWithInput("git", []string{"-C", e.workspace, "apply", "--whitespace=nowarn", "-"}, chunk); chunkErr != nil {
					failed = append(failed, chunkErr.Error())
				} else {
					applied++
				}
			}
			if applied > 0 && len(failed) == 0 {
				return map[string]any{
					"checked":   true,
					"applied":   true,
					"files":     files,
					"fallback":  "chunked",
					"recovered": true,
				}, nil
			}
			return nil, fmt.Errorf("patch apply failed (chunked): applied=%d failed=%d", applied, len(failed))
		}
		return nil, fmt.Errorf("patch apply failed: %w", err)
	}

	return map[string]any{
		"checked": true,
		"applied": true,
		"files":   files,
	}, nil
}
