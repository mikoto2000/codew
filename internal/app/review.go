package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Finding struct {
	Severity string
	Path     string
	Reason   string
}

func ReviewFindings(workspaceRoot string) ([]Finding, []string, error) {
	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, nil, err
	}
	changed, err := changedFiles(workspaceAbs)
	if err != nil {
		return nil, nil, err
	}
	findings := make([]Finding, 0, len(changed))
	for _, p := range changed {
		sev, reason := classifyChange(p)
		findings = append(findings, Finding{Severity: sev, Path: p, Reason: reason})
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if severityRank(findings[i].Severity) == severityRank(findings[j].Severity) {
			return findings[i].Path < findings[j].Path
		}
		return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
	})
	return findings, detectMissingTests(changed), nil
}

func changedFiles(workspace string) ([]string, error) {
	cmd := exec.Command("git", "-C", workspace, "diff", "--name-only", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only failed: %s", strings.TrimSpace(string(out)))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	files := []string{}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			files = append(files, l)
		}
	}
	return files, nil
}

func classifyChange(path string) (string, string) {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "security") || strings.Contains(lower, "auth"):
		return "high", "security/auth related change"
	case strings.HasPrefix(lower, "cmd/") || strings.HasPrefix(lower, "internal/tools/"):
		return "high", "agent/tooling behavior change"
	case strings.HasSuffix(lower, "go.mod") || strings.HasSuffix(lower, "go.sum"):
		return "medium", "dependency or module change"
	case strings.HasSuffix(lower, ".go"):
		return "medium", "application code change"
	default:
		return "low", "non-code/supporting file change"
	}
}

func severityRank(s string) int {
	switch s {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func detectMissingTests(changed []string) []string {
	changedSet := map[string]struct{}{}
	for _, c := range changed {
		changedSet[c] = struct{}{}
	}
	missing := []string{}
	for _, c := range changed {
		if !strings.HasSuffix(c, ".go") || strings.HasSuffix(c, "_test.go") {
			continue
		}
		testFile := strings.TrimSuffix(c, ".go") + "_test.go"
		if _, ok := changedSet[testFile]; !ok {
			missing = append(missing, c)
		}
	}
	sort.Strings(missing)
	return missing
}
