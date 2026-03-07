package tools

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestApplyPatchCheckOnly(t *testing.T) {
	ws := initGitWorkspace(t)
	exec, err := NewExecutor(ws)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}

	patch := "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n"
	raw, _ := json.Marshal(map[string]any{"patch": patch, "check_only": true})

	out, err := exec.applyPatch(raw)
	if err != nil {
		t.Fatalf("applyPatch check_only: %v", err)
	}
	if out["applied"] != false {
		t.Fatalf("expected not applied, got %v", out["applied"])
	}

	data, readErr := os.ReadFile(filepath.Join(ws, "a.txt"))
	if readErr != nil {
		t.Fatalf("read file: %v", readErr)
	}
	if string(data) != "old\n" {
		t.Fatalf("file should be unchanged: %q", string(data))
	}
}

func TestApplyPatchApply(t *testing.T) {
	ws := initGitWorkspace(t)
	exec, err := NewExecutor(ws)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}

	patch := "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n"
	raw, _ := json.Marshal(map[string]any{"patch": patch})

	out, err := exec.applyPatch(raw)
	if err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	if out["applied"] != true {
		t.Fatalf("expected applied=true, got %v", out["applied"])
	}

	data, readErr := os.ReadFile(filepath.Join(ws, "a.txt"))
	if readErr != nil {
		t.Fatalf("read file: %v", readErr)
	}
	if string(data) != "new\n" {
		t.Fatalf("file should be patched: %q", string(data))
	}
}

func initGitWorkspace(t *testing.T) string {
	t.Helper()

	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = ws
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v (%s)", err, string(out))
	}

	return ws
}
