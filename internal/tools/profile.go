package tools

import (
	"fmt"
	"strings"
)

import "ollama-codex-cli/internal/ollama"

const (
	ProfileReadOnly       = "read-only"
	ProfileWorkspaceWrite = "workspace-write"
	ProfileFull           = "full"
)

func NormalizeProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case ProfileReadOnly:
		return ProfileReadOnly
	case ProfileFull:
		return ProfileFull
	default:
		return ProfileWorkspaceWrite
	}
}

func IsToolAllowed(profile, name string) bool {
	profile = NormalizeProfile(profile)
	switch profile {
	case ProfileReadOnly:
		switch name {
		case "list_files", "read_file":
			return true
		default:
			return false
		}
	case ProfileWorkspaceWrite:
		switch name {
		case "list_files", "read_file", "write_file", "replace_in_file", "apply_patch":
			return true
		default:
			return false
		}
	case ProfileFull:
		return true
	default:
		return false
	}
}

func DefinitionsForProfile(profile string) []ollama.ToolDefinition {
	all := Definitions()
	out := make([]ollama.ToolDefinition, 0, len(all))
	for _, d := range all {
		if IsToolAllowed(profile, d.Function.Name) {
			out = append(out, d)
		}
	}
	return out
}

func AllowedToolNamesForProfile(profile string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, def := range DefinitionsForProfile(profile) {
		out[def.Function.Name] = struct{}{}
	}
	return out
}

func CheckShellCommandAllowed(profile, command string) error {
	profile = NormalizeProfile(profile)
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return fmt.Errorf("command is required")
	}
	for _, allowed := range shellCommandAllowlist(profile) {
		if hasPrefix(fields, allowed) {
			return nil
		}
	}
	return fmt.Errorf("shell_exec command is not allowed in profile %q: %s", profile, fields[0])
}

func shellCommandAllowlist(profile string) [][]string {
	switch NormalizeProfile(profile) {
	case ProfileFull:
		return [][]string{
			{"pwd"},
			{"ls"},
			{"find"},
			{"cat"},
			{"sed"},
			{"grep"},
			{"rg"},
			{"git", "status"},
			{"git", "diff"},
			{"git", "show"},
			{"git", "log"},
			{"go", "test"},
			{"go", "fmt"},
			{"go", "vet"},
			{"npm", "test"},
			{"npm", "run"},
			{"pnpm", "test"},
			{"pnpm", "run"},
			{"cargo", "test"},
			{"pytest"},
		}
	default:
		return nil
	}
}

func hasPrefix(fields []string, allowed []string) bool {
	if len(fields) < len(allowed) {
		return false
	}
	for i := range allowed {
		if fields[i] != allowed[i] {
			return false
		}
	}
	return true
}
