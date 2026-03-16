package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mikoto2000/codew/internal/app"
	"github.com/mikoto2000/codew/internal/chatloop"
	"github.com/mikoto2000/codew/internal/contextloader"
	"github.com/mikoto2000/codew/internal/session"
)

var runCmd = &cobra.Command{
	Use:   "run <prompt>",
	Short: "Run a single non-interactive prompt",
	Args:  cobra.ExactArgs(1),
	RunE:  runOnce,
}

func runOnce(cmd *cobra.Command, args []string) (retErr error) {
	prompt := strings.TrimSpace(args[0])
	if prompt == "" {
		return fmt.Errorf("prompt is empty")
	}

	deps, cleanup, err := app.Prepare(func(name string) bool {
		return cmd.Flags().Changed(name)
	}, app.ExecuteOptions{
		ModelProfile:  modelProfile,
		ToolProfile:   toolProfile,
		Model:         chatModel,
		System:        systemText,
		Retries:       retries,
		WorkspaceRoot: workspaceRoot,
		SandboxMode:   sandboxMode,
		DryRun:        dryRun,
		MCPEnabled:    mcpEnabled,
		MCPConfig:     mcpConfig,
		ChatHost:      chatHost,
		Timeout:       timeout,
		ToolLogFile:   toolLogFile,
		TraceLogFile:  traceLogFile,
	})
	if err != nil {
		return err
	}
	defer cleanup()

	s := session.New(deps.Model, buildSystemPrompt(withProjectHint(deps.System, deps.Project), toolsEnabled))
	s.AddUser(prompt)

	autoCtx := ""
	if autoContext {
		ctxText, ctxErr := contextloader.Build(deps.Workspace, prompt, autoContextFiles, autoContextChars)
		if ctxErr == nil {
			autoCtx = ctxText
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
		Client:          deps.Client,
		Executor:        deps.Executor,
		Checkpoints:     deps.Checkpoints,
		ToolLogger:      deps.ToolLogger,
		TraceLogger:     deps.TurnLogger,
		Timeout:         timeout,
		Retries:         deps.Retries,
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
		ToolDefs:     deps.ToolDefs,
		Allowed:      deps.Allowed,
		ToolsEnabled: toolsEnabled,
		Profile:      deps.Profile,
		Sandbox:      deps.Sandbox,
		NetworkAllow: networkAllow,
		NetworkRules: networkRules,
		Workspace:    deps.Workspace,
		AutoContext:  autoCtx,
	})
	finishWorking(err == nil)
	if err != nil {
		return err
	}
	fmt.Println(result.Answer)
	return nil
}
