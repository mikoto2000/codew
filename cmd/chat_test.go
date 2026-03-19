package cmd

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mikoto2000/codew/internal/session"
)

func TestApplyStartupSessionDefaultsUsesPersistedValues(t *testing.T) {
	dir := t.TempDir()
	sessionFile = filepath.Join(dir, "session.json")
	if err := session.SaveToFile(sessionFile, session.Snapshot{
		Host:  "http://example.test:11434",
		Model: "qwen2.5-coder:14b",
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	chatHost = "http://127.0.0.1:11434"
	chatModel = "llama3.2"
	command := cobra.Command{Use: "chat"}
	command.Flags().String("host", chatHost, "")
	command.Flags().String("model", chatModel, "")

	if err := applyStartupSessionDefaults(&command); err != nil {
		t.Fatalf("apply startup defaults: %v", err)
	}

	if chatHost != "http://example.test:11434" {
		t.Fatalf("chatHost = %q", chatHost)
	}
	if chatModel != "qwen2.5-coder:14b" {
		t.Fatalf("chatModel = %q", chatModel)
	}
}

func TestApplyStartupSessionDefaultsKeepsExplicitFlags(t *testing.T) {
	dir := t.TempDir()
	sessionFile = filepath.Join(dir, "session.json")
	if err := session.SaveToFile(sessionFile, session.Snapshot{
		Host:  "http://example.test:11434",
		Model: "qwen2.5-coder:14b",
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	chatHost = "http://manual.test:11434"
	chatModel = "deepseek-coder"
	command := cobra.Command{Use: "chat"}
	command.Flags().String("host", chatHost, "")
	command.Flags().String("model", chatModel, "")
	if err := command.Flags().Set("host", chatHost); err != nil {
		t.Fatalf("set host flag: %v", err)
	}
	if err := command.Flags().Set("model", chatModel); err != nil {
		t.Fatalf("set model flag: %v", err)
	}

	if err := applyStartupSessionDefaults(&command); err != nil {
		t.Fatalf("apply startup defaults: %v", err)
	}

	if chatHost != "http://manual.test:11434" {
		t.Fatalf("chatHost = %q", chatHost)
	}
	if chatModel != "deepseek-coder" {
		t.Fatalf("chatModel = %q", chatModel)
	}
}
