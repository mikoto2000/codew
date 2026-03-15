package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	appVersion       = "dev"
	chatHost         string
	chatModel        string
	systemText       string
	timeout          time.Duration
	toolsEnabled     bool
	autoApprove      bool
	workspaceRoot    string
	maxToolSteps     int
	sessionFile      string
	resumeSession    bool
	autoSave         bool
	maxContextChars  int
	toolProfile      string
	autoValidate     bool
	postEditCmds     []string
	retries          int
	retryBackoff     time.Duration
	fallbackModel    string
	autoContext      bool
	autoContextFiles int
	autoContextChars int
	dryRun           bool
	autoCheckpoint   bool
	toolLog          bool
	toolLogFile      string
	modelProfile     string
	showVersion      bool
	mcpEnabled       bool
	mcpConfig        string
	sandboxMode      string
	networkAllow     bool
	networkAllowTool []string
	traceLog         bool
	traceLogFile     string
)

var rootCmd = &cobra.Command{
	Use:   "codew",
	Short: "Codex CLI style client for Ollama",
	Long:  "A Codex CLI-style assistant that talks to an Ollama server via /api/chat.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println(appVersion)
			os.Exit(0)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runChat(cmd, args)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&chatHost, "host", getEnv("OLLAMA_HOST", "http://127.0.0.1:11434"), "Ollama API host")
	rootCmd.PersistentFlags().StringVar(&chatModel, "model", getEnv("OLLAMA_MODEL", "llama3.2"), "Default model name")
	rootCmd.PersistentFlags().StringVar(&systemText, "system", getEnv("OLLAMA_SYSTEM", "You are a coding assistant."), "System prompt")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 120*time.Second, "HTTP request timeout")
	rootCmd.PersistentFlags().BoolVar(&toolsEnabled, "tools", true, "Enable tool calling")
	rootCmd.PersistentFlags().BoolVar(&autoApprove, "auto-approve", false, "Auto-approve all tool calls (unsafe in untrusted workspaces)")
	rootCmd.PersistentFlags().StringVar(&workspaceRoot, "workspace", ".", "Workspace root for tool access")
	rootCmd.PersistentFlags().IntVar(&maxToolSteps, "max-tool-steps", 8, "Max tool-calling rounds per user turn")
	rootCmd.PersistentFlags().StringVar(&sessionFile, "session-file", ".codew/session.json", "Path for session save/load")
	rootCmd.PersistentFlags().BoolVar(&resumeSession, "resume", false, "Load previous session from session-file on startup")
	rootCmd.PersistentFlags().BoolVar(&autoSave, "auto-save", true, "Auto-save session after each turn")
	rootCmd.PersistentFlags().IntVar(&maxContextChars, "max-context-chars", 24000, "Approximate max characters sent as chat context")
	rootCmd.PersistentFlags().StringVar(&toolProfile, "tool-profile", "workspace-write", "Tool permission profile: read-only | workspace-write | full")
	rootCmd.PersistentFlags().BoolVar(&autoValidate, "auto-validate", false, "Run post-edit validation commands after successful edit tools")
	rootCmd.PersistentFlags().StringSliceVar(&postEditCmds, "post-edit-cmd", []string{"go test ./..."}, "Validation command(s) to run after edit tools")
	rootCmd.PersistentFlags().IntVar(&retries, "retries", 2, "Retry count per model when API request fails")
	rootCmd.PersistentFlags().DurationVar(&retryBackoff, "retry-backoff", 2*time.Second, "Base backoff duration between retries")
	rootCmd.PersistentFlags().StringVar(&fallbackModel, "fallback-model", "", "Fallback model to use after retries are exhausted")
	rootCmd.PersistentFlags().BoolVar(&autoContext, "auto-context", true, "Auto-load relevant project files into prompt context")
	rootCmd.PersistentFlags().IntVar(&autoContextFiles, "auto-context-files", 4, "Max number of files to auto-load as context per turn")
	rootCmd.PersistentFlags().IntVar(&autoContextChars, "auto-context-chars", 8000, "Max total characters for auto-loaded context per turn")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Do not apply edit tools, only show edit plan")
	rootCmd.PersistentFlags().BoolVar(&autoCheckpoint, "auto-checkpoint", true, "Create checkpoint before first edit tool in each turn")
	rootCmd.PersistentFlags().BoolVar(&toolLog, "tool-log", true, "Write tool execution logs in JSONL format")
	rootCmd.PersistentFlags().StringVar(&toolLogFile, "tool-log-file", ".codew/tool_logs.jsonl", "Path to tool execution JSONL log file")
	rootCmd.PersistentFlags().StringVar(&modelProfile, "model-profile", "", "Preset profile: coding-fast | coding-safe | research")
	rootCmd.PersistentFlags().BoolVar(&showVersion, "version", false, "Show version and exit")
	rootCmd.PersistentFlags().BoolVar(&mcpEnabled, "mcp", false, "Enable MCP client tools from config")
	rootCmd.PersistentFlags().StringVar(&mcpConfig, "mcp-config", ".codew/mcp.json", "Path to MCP server config JSON")
	rootCmd.PersistentFlags().StringVar(&sandboxMode, "sandbox-mode", "workspace-write", "Sandbox permission mode: read-only | workspace-write | full")
	rootCmd.PersistentFlags().BoolVar(&networkAllow, "network-allow", false, "Allow network tool escalation for this session")
	rootCmd.PersistentFlags().StringSliceVar(&networkAllowTool, "network-allow-tool", nil, "Allow network escalation for specific tool names")
	rootCmd.PersistentFlags().BoolVar(&traceLog, "trace-log", true, "Write turn-level execution traces")
	rootCmd.PersistentFlags().StringVar(&traceLogFile, "trace-log-file", ".codew/turn_traces.jsonl", "Path to turn trace JSONL file")

	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(reviewCmd)
}
