package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type doctorResult struct {
	Name    string
	OK      bool
	Details string
}

type tagsResp struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run local diagnostics for codew",
	RunE:  runDoctor,
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	results := []doctorResult{}

	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return err
	}

	results = append(results, checkOllama())
	results = append(results, checkModelExists())
	results = append(results, checkWorkspaceWrite(workspaceAbs))
	results = append(results, checkGitState(workspaceAbs))

	failed := 0
	for _, r := range results {
		status := "OK"
		if !r.OK {
			status = "NG"
			failed++
		}
		fmt.Printf("[%s] %s: %s\n", status, r.Name, r.Details)
	}

	if failed > 0 {
		return fmt.Errorf("doctor found %d issue(s)", failed)
	}
	fmt.Println("All checks passed.")
	return nil
}

func checkOllama() doctorResult {
	url := strings.TrimRight(chatHost, "/") + "/api/tags"
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return doctorResult{Name: "ollama_connect", OK: false, Details: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return doctorResult{Name: "ollama_connect", OK: false, Details: fmt.Sprintf("%s %s", resp.Status, strings.TrimSpace(string(body)))}
	}
	return doctorResult{Name: "ollama_connect", OK: true, Details: url}
}

func checkModelExists() doctorResult {
	url := strings.TrimRight(chatHost, "/") + "/api/tags"
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return doctorResult{Name: "model_exists", OK: false, Details: err.Error()}
	}
	defer resp.Body.Close()

	var payload tagsResp
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return doctorResult{Name: "model_exists", OK: false, Details: err.Error()}
	}
	for _, m := range payload.Models {
		if m.Name == chatModel {
			return doctorResult{Name: "model_exists", OK: true, Details: chatModel}
		}
	}
	return doctorResult{Name: "model_exists", OK: false, Details: fmt.Sprintf("%s not found", chatModel)}
}

func checkWorkspaceWrite(workspace string) doctorResult {
	tmpDir := filepath.Join(workspace, ".codew")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return doctorResult{Name: "workspace_write", OK: false, Details: err.Error()}
	}
	path := filepath.Join(tmpDir, ".doctor_write_test")
	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		return doctorResult{Name: "workspace_write", OK: false, Details: err.Error()}
	}
	_ = os.Remove(path)
	return doctorResult{Name: "workspace_write", OK: true, Details: workspace}
}

func checkGitState(workspace string) doctorResult {
	cmd := exec.Command("git", "-C", workspace, "rev-parse", "--is-inside-work-tree")
	if out, err := cmd.CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "true" {
		return doctorResult{Name: "git_state", OK: false, Details: "not a git repository"}
	}
	statusCmd := exec.Command("git", "-C", workspace, "status", "--porcelain")
	out, err := statusCmd.CombinedOutput()
	if err != nil {
		return doctorResult{Name: "git_state", OK: false, Details: err.Error()}
	}
	lines := strings.TrimSpace(string(out))
	if lines == "" {
		return doctorResult{Name: "git_state", OK: true, Details: "clean"}
	}
	count := len(strings.Split(lines, "\n"))
	return doctorResult{Name: "git_state", OK: true, Details: fmt.Sprintf("dirty (%d changes)", count)}
}
