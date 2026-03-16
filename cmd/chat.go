package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/peterh/liner"
	"github.com/spf13/cobra"

	"github.com/mikoto2000/codew/internal/agent"
	"github.com/mikoto2000/codew/internal/app"
	"github.com/mikoto2000/codew/internal/chatloop"
	"github.com/mikoto2000/codew/internal/contextloader"
	"github.com/mikoto2000/codew/internal/logging"
	"github.com/mikoto2000/codew/internal/ollama"
	"github.com/mikoto2000/codew/internal/plan"
	"github.com/mikoto2000/codew/internal/session"
	"github.com/mikoto2000/codew/internal/tools"
)

const toolPromptSuffix = `
If you need to use a tool, respond with JSON only (no markdown), using one of these formats:
{"name":"tool_name","arguments":{...}}
{"tool_calls":[{"name":"tool_name","arguments":{...}}]}
For file edits, prefer apply_patch over full-file overwrite when possible.
When web_search results are used, include source URLs in your final answer.
After receiving tool results, provide a normal final answer for the user.
`

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session",
	RunE:  runChat,
}

func runChat(cmd *cobra.Command, _ []string) error {
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
	workspaceAbs := deps.Workspace
	profile := deps.Profile
	sandbox := deps.Sandbox
	checkpoints := deps.Checkpoints
	sessionPath, err := filepath.Abs(sessionFile)
	if err != nil {
		return fmt.Errorf("resolve session-file: %w", err)
	}
	logPath := toolLogFile
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(workspaceAbs, logPath)
	}
	tracePath := traceLogFile
	if !filepath.IsAbs(tracePath) {
		tracePath = filepath.Join(workspaceAbs, tracePath)
	}
	historyPath := filepath.Join(workspaceAbs, ".codew", "history.txt")

	s := session.New(deps.Model, buildSystemPrompt(withProjectHint(deps.System, deps.Project), toolsEnabled))
	if resumeSession {
		resumed, loadErr := chatloop.ResumeSession(sessionPath, s)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to resume session: %v\n", loadErr)
		} else if resumed {
			fmt.Println(chatloop.ResumeMessage(sessionPath))
		}
	}
	toolDefs := deps.ToolDefs
	allowed := deps.Allowed

	fmt.Printf("Connected target: %s\n", chatHost)
	fmt.Printf("Project type: %s (%s)\n", deps.Project.Primary, strings.Join(deps.Project.All, ","))
	fmt.Printf("Model: %s\n", s.Model)
	if modelProfile != "" {
		fmt.Printf("Model profile: %s\n", modelProfile)
	}
	fmt.Printf("Tools: %t (auto-approve=%t)\n", toolsEnabled, autoApprove)
	fmt.Printf("Tool profile: %s\n", profile)
	fmt.Printf("Sandbox mode: %s\n", sandbox)
	if autoApprove {
		fmt.Println("Warning: auto-approve skips mutating tool confirmation prompts.")
	}
	fmt.Printf("Network escalation: allow=%t allow-tools=%v\n", networkAllow, networkAllowTool)
	fmt.Printf("MCP: %t (config=%s, tools=%d)\n", mcpEnabled, mcpConfig, len(deps.MCPManager.Definitions()))
	fmt.Printf("Context limit: %d chars\n", maxContextChars)
	fmt.Printf("Auto context: %t (files=%d chars=%d)\n", autoContext, autoContextFiles, autoContextChars)
	fmt.Printf("Dry run: %t\n", dryRun)
	fmt.Printf("Auto checkpoint: %t\n", autoCheckpoint)
	fmt.Printf("Tool log: %t (%s)\n", toolLog, logPath)
	fmt.Printf("Trace log: %t (%s)\n", traceLog, tracePath)
	fmt.Printf("Retries: %d (backoff=%s, fallback=%s)\n", deps.Retries, retryBackoff, fallbackModel)
	fmt.Printf("Session file: %s (auto-save=%t)\n", sessionPath, autoSave)
	fmt.Println("Commands: /exit, /model <name>, /models, /system <text>, /reset, /save, /load, /checkpoint, /undo, /help")

	lineEditor := liner.NewLiner()
	defer lineEditor.Close()
	lineEditor.SetCtrlCAborts(true)
	if err := chatloop.LoadLineHistory(lineEditor, historyPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load history: %v\n", err)
	}
	defer func() {
		if err := chatloop.SaveLineHistory(lineEditor, historyPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to save history: %v\n", err)
		}
	}()

	approveAll := autoApprove
	networkRules := map[string]bool{}
	planner := plan.New()
	for _, t := range networkAllowTool {
		t = strings.TrimSpace(t)
		if t != "" {
			networkRules[t] = true
		}
	}

	for {
		line, err := lineEditor.Prompt("you> ")
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if errors.Is(err, liner.ErrPromptAborted) {
				continue
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lineEditor.AppendHistory(line)

		if strings.HasPrefix(line, "/") {
			done, cmdErr := chatloop.ExecuteCommand(cmd.Context(), line, s, checkpoints, planner, deps.Client, chatloop.CommandOptions{
				SessionPath:       sessionPath,
				ToolsEnabled:      toolsEnabled,
				Timeout:           timeout,
				BuildSystemPrompt: buildSystemPrompt,
			})
			if cmdErr != nil {
				fmt.Fprintf(os.Stderr, "command error: %v\n", cmdErr)
			} else if autoSave {
				if saveErr := chatloop.SaveSessionSnapshot(sessionPath, s); saveErr != nil {
					fmt.Fprintf(os.Stderr, "warning: auto-save failed: %v\n", saveErr)
				}
			}
			if done {
				return nil
			}
			continue
		}

		s.AddUser(line)
		turnStart := time.Now()
		turnID := strconv.FormatInt(turnStart.UnixNano(), 10)
		turnToolCalls := 0
		turnErr := ""
		if traceLog {
			_ = deps.TurnLogger.Append(logging.TraceEvent{
				Event:  "turn_started",
				TurnID: turnID,
				Mode:   "chat",
				Input:  line,
				Model:  s.Model,
			})
		}
		autoCtx := ""
		if autoContext {
			ctxText, ctxErr := contextloader.Build(workspaceAbs, line, autoContextFiles, autoContextChars)
			if ctxErr != nil {
				fmt.Fprintf(os.Stderr, "warning: auto-context failed: %v\n", ctxErr)
			} else {
				autoCtx = ctxText
				if autoCtx != "" {
					fmt.Printf("[auto-context] loaded\n")
				}
			}
		}
		finalPrinted := false
		turnCheckpointed := false
		for step := 0; step < maxToolSteps; step++ {
			messages := chatloop.WithAutoContext(s.MessagesForModel(maxContextChars), autoCtx)
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			finishWorking := announceWorking("assistant is working")
			msg, usedModel, chatErr := chatWithRetry(ctx, deps.Client, s.Model, messages, toolDefs)
			cancel()
			finishWorking(chatErr == nil)

			if chatErr != nil {
				fmt.Fprintf(os.Stderr, "\nrequest failed: %v\n", chatErr)
				turnErr = chatErr.Error()
				if step == 0 {
					s.RollbackLastUser()
				}
				if usedModel != s.Model {
					fmt.Printf("\n[model fallback] using %s\n", usedModel)
				}
				break
			}
			if traceLog {
				_ = deps.TurnLogger.Append(logging.TraceEvent{
					Event:  "model_response_received",
					TurnID: turnID,
					Step:   step + 1,
					Mode:   "chat",
					Model:  usedModel,
				})
			}

			parseResult := agent.ExtractToolCalls(msg, allowed)
			toolCalls, parsed := parseResult.Calls, parseResult.Parsed
			if parsed && len(toolCalls) == 0 && len(parseResult.Diagnostics) > 0 {
				fmt.Fprintf(os.Stderr, "[toolparse] %s\n", agent.FormatDiagnostics(parseResult.Diagnostics))
			}
			if toolsEnabled && len(toolCalls) > 0 {
				if traceLog {
					for _, call := range toolCalls {
						_ = deps.TurnLogger.Append(logging.TraceEvent{
							Event:      "tool_call_parsed",
							TurnID:     turnID,
							Step:       step + 1,
							ToolCallID: call.ID,
							Tool:       call.Function.Name,
							Mode:       "chat",
						})
					}
				}
				msg.ToolCalls = toolCalls
				s.AddAssistantMessage(msg)
				if chatloop.CanOrchestrateInParallel(toolCalls, profile, sandbox, networkAllow, networkRules) {
					results := chatloop.RunToolCallsOrchestrated(deps.Executor, toolCalls, sandbox)
					for i, call := range toolCalls {
						toolResult := results[i]
						s.AddTool(call.Function.Name, call.ID, toolResult)
						fmt.Printf("\n[tool:%s] %s\n", call.Function.Name, chatloop.SummarizeToolResult(toolResult))
						writeToolLog(deps.ToolLogger, line, call.Function.Name, chatloop.CompactJSON(call.Function.Arguments), toolResult, true)
						if traceLog {
							_ = deps.TurnLogger.Append(logging.TraceEvent{
								Event:      "tool_call_executed",
								TurnID:     turnID,
								Step:       step + 1,
								ToolCallID: call.ID,
								Tool:       call.Function.Name,
								Mode:       "chat",
							})
						}
						turnToolCalls++
					}
					continue
				}

				for _, call := range toolCalls {
					decision := chatloop.Decide(chatloop.ApprovalRequest{
						ToolName:     call.Function.Name,
						IsMCP:        deps.MCPManager != nil && deps.MCPManager.HasTool(call.Function.Name),
						IsMutating:   tools.IsMutatingTool(call.Function.Name),
						Sandbox:      sandbox,
						AutoApprove:  approveAll,
						NetworkAllow: networkAllow,
						NetworkRules: networkRules,
						Profile:      profile,
					})
					if decision == chatloop.DecisionDenied {
						toolResult := `{"ok":false,"error":"tool call denied by policy"}`
						s.AddTool(call.Function.Name, call.ID, toolResult)
						fmt.Printf("\n[tool:%s] denied\n", call.Function.Name)
						writeToolLog(deps.ToolLogger, line, call.Function.Name, chatloop.CompactJSON(call.Function.Arguments), toolResult, false)
						if traceLog {
							_ = deps.TurnLogger.Append(logging.TraceEvent{
								Event:      "tool_call_denied",
								TurnID:     turnID,
								Step:       step + 1,
								ToolCallID: call.ID,
								Tool:       call.Function.Name,
								Mode:       "chat",
								Error:      "tool call denied by policy",
							})
						}
						turnToolCalls++
						continue
					}
					if decision == chatloop.DecisionNeedsUserApproval {
						preview := ""
						if tools.IsMutatingTool(call.Function.Name) {
							preview = tools.Preview(call)
						}
						approved, allowAll := chatloop.AskToolApproval(lineEditor, call, preview)
						if allowAll {
							approveAll = true
						}
						if !approved {
							toolResult := `{"ok":false,"error":"tool call rejected by user"}`
							s.AddTool(call.Function.Name, call.ID, toolResult)
							fmt.Printf("\n[tool:%s] rejected\n", call.Function.Name)
							writeToolLog(deps.ToolLogger, line, call.Function.Name, chatloop.CompactJSON(call.Function.Arguments), toolResult, false)
							if traceLog {
								_ = deps.TurnLogger.Append(logging.TraceEvent{
									Event:      "tool_call_denied",
									TurnID:     turnID,
									Step:       step + 1,
									ToolCallID: call.ID,
									Tool:       call.Function.Name,
									Mode:       "chat",
									Error:      "tool call rejected by user",
								})
							}
							turnToolCalls++
							continue
						}
					}

					fmt.Printf("\n[tool:%s] running...\n", call.Function.Name)
					callSandbox := sandbox
					if decision == chatloop.DecisionNeedsNetworkEscalation {
						allowOnce, allowAlways := chatloop.AskNetworkEscalation(lineEditor, call.Function.Name)
						if allowAlways {
							networkRules[call.Function.Name] = true
						}
						if !allowOnce && !allowAlways {
							toolResult := `{"ok":false,"error":"network escalation denied by user"}`
							s.AddTool(call.Function.Name, call.ID, toolResult)
							fmt.Printf("[tool:%s] denied network escalation\n", call.Function.Name)
							writeToolLog(deps.ToolLogger, line, call.Function.Name, chatloop.CompactJSON(call.Function.Arguments), toolResult, false)
							if traceLog {
								_ = deps.TurnLogger.Append(logging.TraceEvent{
									Event:      "tool_call_denied",
									TurnID:     turnID,
									Step:       step + 1,
									ToolCallID: call.ID,
									Tool:       call.Function.Name,
									Mode:       "chat",
									Error:      "network escalation denied by user",
								})
							}
							continue
						}
						callSandbox = tools.SandboxFull
					}
					if autoCheckpoint && !turnCheckpointed && tools.IsMutatingTool(call.Function.Name) && !dryRun {
						id, cpErr := checkpoints.Create()
						if cpErr != nil {
							fmt.Fprintf(os.Stderr, "[checkpoint] failed: %v\n", cpErr)
						} else {
							turnCheckpointed = true
							fmt.Printf("[checkpoint] created: %s\n", id)
							if traceLog {
								_ = deps.TurnLogger.Append(logging.TraceEvent{
									Event:  "checkpoint_created",
									TurnID: turnID,
									Step:   step + 1,
									Tool:   call.Function.Name,
									Mode:   "chat",
								})
							}
						}
					}
					toolResult := deps.Executor.ExecuteWithSandbox(call, callSandbox)
					s.AddTool(call.Function.Name, call.ID, toolResult)
					fmt.Printf("[tool:%s] %s\n", call.Function.Name, chatloop.SummarizeToolResult(toolResult))
					writeToolLog(deps.ToolLogger, line, call.Function.Name, chatloop.CompactJSON(call.Function.Arguments), toolResult, true)
					if traceLog {
						_ = deps.TurnLogger.Append(logging.TraceEvent{
							Event:      "tool_call_executed",
							TurnID:     turnID,
							Step:       step + 1,
							ToolCallID: call.ID,
							Tool:       call.Function.Name,
							Mode:       "chat",
						})
					}
					turnToolCalls++
					if autoValidate && tools.IsMutatingTool(call.Function.Name) && toolCallSucceeded(toolResult) {
						validateResult := runValidation(workspaceAbs, postEditCmds)
						s.AddTool("post_validate", "", validateResult)
						fmt.Printf("[post-validate] %s\n", chatloop.SummarizeToolResult(validateResult))
						writeToolLog(deps.ToolLogger, line, "post_validate", strings.Join(postEditCmds, " && "), validateResult, true)
						if traceLog {
							_ = deps.TurnLogger.Append(logging.TraceEvent{
								Event:  "post_validate_finished",
								TurnID: turnID,
								Step:   step + 1,
								Tool:   call.Function.Name,
								Mode:   "chat",
							})
						}
						turnToolCalls++
					}
				}

				if parsed && strings.TrimSpace(msg.Content) != "" {
					// JSON was consumed as a tool call; do not print it as a user-facing reply.
				}
				continue
			}

			answer := strings.TrimSpace(msg.Content)
			s.AddAssistantMessage(msg)
			if answer != "" {
				fmt.Printf("assistant> %s", answer)
				finalPrinted = true
			}
			break
		}

		if !finalPrinted {
			fmt.Print("(no response)")
		}
		fmt.Println()
		if traceLog {
			if !finalPrinted {
				turnErr = "no_response"
			}
			_ = deps.TurnLogger.Append(logging.TraceEvent{
				Event:      "turn_finished",
				TurnID:     turnID,
				Mode:       "chat",
				Input:      line,
				DurationMS: time.Since(turnStart).Milliseconds(),
				ToolCalls:  turnToolCalls,
				Error:      turnErr,
			})
		}
		if autoSave {
			if saveErr := chatloop.SaveSessionSnapshot(sessionPath, s); saveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: auto-save failed: %v\n", saveErr)
			}
		}
	}
}

func toolCallSucceeded(raw string) bool {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return false
	}
	ok, _ := obj["ok"].(bool)
	return ok
}

func runValidation(workspace string, commands []string) string {
	result := map[string]any{
		"tool":     "post_validate",
		"ok":       true,
		"commands": []map[string]any{},
	}
	entries := make([]map[string]any, 0, len(commands))
	for _, command := range commands {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		c := exec.CommandContext(ctx, "bash", "-lc", command)
		c.Dir = workspace
		out, err := c.CombinedOutput()
		cancel()

		entry := map[string]any{
			"command": command,
			"output":  truncateOutput(string(out), 2000),
		}
		if err != nil {
			entry["error"] = err.Error()
			result["ok"] = false
		}
		entries = append(entries, entry)
	}
	result["commands"] = entries

	data, _ := json.Marshal(result)
	return string(data)
}

func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...<truncated>"
}

func writeToolLog(logger *logging.ToolLogger, turnInput, toolName, args, result string, approved bool) {
	if !toolLog || logger == nil {
		return
	}
	if err := logger.Append(logging.ToolEvent{
		TurnInput: turnInput,
		Tool:      toolName,
		Args:      args,
		Result:    result,
		Approved:  approved,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: tool log failed: %v\n", err)
	}
}

func chatWithRetry(ctx context.Context, client *ollama.Client, primaryModel string, messages []ollama.Message, defs []ollama.ToolDefinition) (ollama.Message, string, error) {
	models := []string{primaryModel}
	if strings.TrimSpace(fallbackModel) != "" && fallbackModel != primaryModel {
		models = append(models, fallbackModel)
	}

	var lastErr error
	for modelIndex, model := range models {
		for attempt := 0; attempt <= retries; attempt++ {
			msg, err := client.Chat(ctx, model, messages, defs)
			if err == nil {
				return msg, model, nil
			}
			lastErr = err
			if attempt < retries {
				sleep := retryBackoff * time.Duration(1<<attempt)
				select {
				case <-time.After(sleep):
				case <-ctx.Done():
					return ollama.Message{}, model, ctx.Err()
				}
				continue
			}
		}
		if modelIndex < len(models)-1 {
			fmt.Printf("\n[retry] model %s exhausted, switching to %s\n", model, models[modelIndex+1])
		}
	}
	return ollama.Message{}, primaryModel, lastErr
}

func buildSystemPrompt(base string, enableTools bool) string {
	if !enableTools {
		return base
	}
	return strings.TrimSpace(base) + "\n\n" + strings.TrimSpace(toolPromptSuffix)
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
