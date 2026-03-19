package codewconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingConfig(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.ShellAllow) != 0 {
		t.Fatalf("ShellAllow = %#v", cfg.ShellAllow)
	}
}

func TestLoadShellAllow(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".codew")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data := []byte("{\"shell_allow\":[\"terraform plan\",\"docker compose ps\"]}")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(ws)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"terraform plan", "docker compose ps"}
	if !reflect.DeepEqual(cfg.ShellAllow, want) {
		t.Fatalf("ShellAllow = %#v, want %#v", cfg.ShellAllow, want)
	}
}
