package cmd

import (
	"strings"

	"ollama-codex-cli/internal/projectdetect"
)

func withProjectHint(base string, result projectdetect.Result) string {
	hint := "Detected project types: " + strings.Join(result.All, ", ")
	if strings.TrimSpace(base) == "" {
		return hint
	}
	return strings.TrimSpace(base) + "\n\n" + hint
}
