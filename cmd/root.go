package cmd

import (
	"time"

	"github.com/spf13/cobra"
)

var (
	chatHost      string
	chatModel     string
	systemText    string
	timeout       time.Duration
	toolsEnabled  bool
	autoApprove   bool
	workspaceRoot string
	maxToolSteps  int
)

var rootCmd = &cobra.Command{
	Use:   "ocli",
	Short: "Codex CLI style client for Ollama",
	Long:  "A Codex CLI-style assistant that talks to an Ollama server via /api/chat.",
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
	rootCmd.PersistentFlags().BoolVar(&autoApprove, "auto-approve", false, "Auto-approve all tool calls")
	rootCmd.PersistentFlags().StringVar(&workspaceRoot, "workspace", ".", "Workspace root for tool access")
	rootCmd.PersistentFlags().IntVar(&maxToolSteps, "max-tool-steps", 8, "Max tool-calling rounds per user turn")

	rootCmd.AddCommand(chatCmd)
}
