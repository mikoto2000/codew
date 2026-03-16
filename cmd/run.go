package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"ollama-codex-cli/internal/chatloop"
	"ollama-codex-cli/internal/checkpoint"
	"ollama-codex-cli/internal/contextloader"
	"ollama-codex-cli/internal/logging"
	"ollama-codex-cli/internal/mcp"
	"ollama-codex-cli/internal/modelprofile"
	"ollama-codex-cli/internal/ollama"
	"ollama-codex-cli/internal/projectdetect"
	"ollama-codex-cli/internal/session"
	"ollama-codex-cli/internal/tools"
)

var runCmd = &cobra.Command{
	Use:   "run <prompt>",
	Short: "Run a single non-interactive prompt",
	Args:  cobra.ExactArgs(1),
	RunE:  runOnce,
}

func runOnce(cmd *cobra.Command, args []string) (retErr error) {
	if err := modelprofile.Apply(modelProfile, &chatModel, &systemText, &toolProfile, &retries, func(name string) bool {
		return cmd.Flags().Changed(name)
	}); err != nil {
		return err
	}

	prompt := strings.TrimSpace(args[0])
	if prompt == "" {
		return fmt.Errorf("prompt is empty")
	}

	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return err
	}
	project := projectdetect.Detect(workspaceAbs)
	profile := tools.NormalizeProfile(toolProfile)
	sandbox := tools.NormalizeSandboxMode(sandboxMode)
	client := ollama.NewClient(chatHost, timeout)
	mcpManager := mcp.NewManager()
	if mcpEnabled {
		mcpCtx, cancelMCP := context.WithTimeout(cmd.Context(), timeout)
		err = mcpManager.LoadAndStart(mcpCtx, workspaceAbs, mcpConfig)
		cancelMCP()
		if err != nil {
			return fmt.Errorf("load mcp tools: %w", err)
		}
		defer mcpManager.Close()
	}
	executor, err := tools.NewExecutor(workspaceAbs, profile, dryRun, sandbox, mcpManager)
	if err != nil {
		return err
	}
	cp := checkpoint.New(workspaceAbs)

	logPath := toolLogFile
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(workspaceAbs, logPath)
	}
	toolLogger := logging.NewToolLogger(logPath)
	tracePath := traceLogFile
	if !filepath.IsAbs(tracePath) {
		tracePath = filepath.Join(workspaceAbs, tracePath)
	}
	turnLogger := logging.NewTurnLogger(tracePath)

	s := session.New(chatModel, buildSystemPrompt(withProjectHint(systemText, project), toolsEnabled))
	s.AddUser(prompt)

	autoCtx := ""
	if autoContext {
		ctxText, ctxErr := contextloader.Build(workspaceAbs, prompt, autoContextFiles, autoContextChars)
		if ctxErr == nil {
			autoCtx = ctxText
		}
	}

	toolDefs := []ollama.ToolDefinition(nil)
	allowed := map[string]struct{}{}
	if toolsEnabled {
		toolDefs = tools.DefinitionsForProfile(profile)
		toolDefs = append(toolDefs, mcpManager.Definitions()...)
		allowed = tools.AllowedToolNamesForProfile(profile)
		for _, def := range mcpManager.Definitions() {
			allowed[def.Function.Name] = struct{}{}
		}
	}

	networkRules := map[string]bool{}
	for _, t := range networkAllowTool {
		t = strings.TrimSpace(t)
		if t != "" {
			networkRules[t] = true
		}
	}
	runner := chatloop.Runner{
		Client:          client,
		Executor:        executor,
		Checkpoints:     cp,
		ToolLogger:      toolLogger,
		TraceLogger:     turnLogger,
		Timeout:         timeout,
		Retries:         retries,
		RetryBackoff:    retryBackoff,
		FallbackModel:   fallbackModel,
		MaxToolSteps:    maxToolSteps,
		MaxContextChars: maxContextChars,
		AutoCheckpoint:  autoCheckpoint,
		DryRun:          dryRun,
		AutoValidate:    autoValidate,
		PostEditCmds:    postEditCmds,
		ToolLogEnabled:  toolLog,
		TraceLogEnabled: traceLog,
	}
	finishWorking := announceWorking("assistant is working")
	result, err := runner.Execute(cmd.Context(), chatloop.RunRequest{
		Mode:         "run",
		Prompt:       prompt,
		Session:      s,
		ToolDefs:     toolDefs,
		Allowed:      allowed,
		ToolsEnabled: toolsEnabled,
		Profile:      profile,
		Sandbox:      sandbox,
		NetworkAllow: networkAllow,
		NetworkRules: networkRules,
		Workspace:    workspaceAbs,
		AutoContext:  autoCtx,
	})
	finishWorking(err == nil)
	if err != nil {
		return err
	}
	fmt.Println(result.Answer)
	return nil
}
