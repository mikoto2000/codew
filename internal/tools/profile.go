package tools

import "strings"

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
