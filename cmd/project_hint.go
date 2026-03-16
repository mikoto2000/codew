package cmd

import (
	"strings"

	"github.com/mikoto2000/codew/internal/projectdetect"
)

func withProjectHint(base string, result projectdetect.Result) string {
	hint := "Detected project types: " + strings.Join(result.All, ", ")
	if strings.TrimSpace(base) == "" {
		return hint
	}
	return strings.TrimSpace(base) + "\n\n" + hint
}
