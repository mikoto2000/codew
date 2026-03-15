package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ollama-codex-cli/internal/agent"
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
	turnStart := time.Now()
	turnToolCalls := 0
	defer func() {
		if traceLog {
			errMsg := ""
			if retErr != nil {
				errMsg = retErr.Error()
			}
			_ = turnLogger.Append(logging.TurnEvent{
				Mode:       "run",
				Input:      prompt,
				DurationMS: time.Since(turnStart).Milliseconds(),
				ToolCalls:  turnToolCalls,
				Error:      errMsg,
			})
		}
	}()

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

	checkpointed := false
	networkRules := map[string]bool{}
	for _, t := range networkAllowTool {
		t = strings.TrimSpace(t)
		if t != "" {
			networkRules[t] = true
		}
	}
	for step := 0; step < maxToolSteps; step++ {
		messages := withAutoContext(s.MessagesForModel(maxContextChars), autoCtx)
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		finishWorking := announceWorking("assistant is working")
		msg, _, chatErr := chatWithRetry(ctx, client, s.Model, messages, toolDefs)
		cancel()
		finishWorking(chatErr == nil)
		if chatErr != nil {
			return chatErr
		}

		parseResult := agent.ExtractToolCalls(msg, allowed)
		toolCalls := parseResult.Calls
		if parseResult.Parsed && len(toolCalls) == 0 && len(parseResult.Diagnostics) > 0 {
			fmt.Fprintf(os.Stderr, "[toolparse] %s\n", agent.FormatDiagnostics(parseResult.Diagnostics))
		}
		if toolsEnabled && len(toolCalls) > 0 {
			s.AddAssistantMessage(msg)
			results := map[int]string{}
			if canOrchestrateInParallel(toolCalls, profile, sandbox, networkAllow, networkRules) {
				parallel := runToolCallsOrchestrated(executor, toolCalls, sandbox)
				for i, result := range parallel {
					results[i] = result
				}
			} else {
				for i, call := range toolCalls {
					callSandbox := sandbox
					decision := chatloop.Decide(chatloop.ApprovalRequest{
						ToolName:     call.Function.Name,
						IsMCP:        mcpManager != nil && mcpManager.HasTool(call.Function.Name),
						IsMutating:   tools.IsMutatingTool(call.Function.Name),
						Sandbox:      sandbox,
						AutoApprove:  autoApprove,
						NetworkAllow: networkAllow,
						NetworkRules: networkRules,
						Profile:      profile,
					})
					if decision == chatloop.DecisionDenied {
						results[i] = `{"ok":false,"error":"tool call denied by policy"}`
						continue
					}
					if decision == chatloop.DecisionNeedsNetworkEscalation {
						if networkAllow || networkRules[call.Function.Name] {
							callSandbox = tools.SandboxFull
						}
					}
					if autoCheckpoint && !checkpointed && tools.IsMutatingTool(call.Function.Name) && !dryRun {
						if id, e := cp.Create(); e == nil {
							checkpointed = true
							fmt.Fprintf(os.Stderr, "[checkpoint] created: %s\n", id)
						}
					}
					results[i] = executor.ExecuteWithSandbox(call, callSandbox)
				}
			}

			for i, call := range toolCalls {
				res := results[i]
				s.AddTool(call.Function.Name, call.ID, res)
				writeToolLog(toolLogger, prompt, call.Function.Name, compactJSON(call.Function.Arguments), res, true)
				turnToolCalls++
				if autoValidate && tools.IsMutatingTool(call.Function.Name) && toolCallSucceeded(res) {
					validateResult := runValidation(workspaceAbs, postEditCmds)
					s.AddTool("post_validate", "", validateResult)
					writeToolLog(toolLogger, prompt, "post_validate", strings.Join(postEditCmds, " && "), validateResult, true)
					turnToolCalls++
				}
			}
			continue
		}

		answer := strings.TrimSpace(msg.Content)
		s.AddAssistantMessage(msg)
		fmt.Println(answer)
		return nil
	}
	return fmt.Errorf("max-tool-steps reached without final response")
}
