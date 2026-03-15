package cmd

import (
	"bytes"
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
	"sync"
	"time"

	"github.com/peterh/liner"
	"github.com/spf13/cobra"

	"ollama-codex-cli/internal/agent"
	"ollama-codex-cli/internal/chatloop"
	"ollama-codex-cli/internal/checkpoint"
	"ollama-codex-cli/internal/contextloader"
	"ollama-codex-cli/internal/logging"
	"ollama-codex-cli/internal/mcp"
	"ollama-codex-cli/internal/modelprofile"
	"ollama-codex-cli/internal/ollama"
	"ollama-codex-cli/internal/plan"
	"ollama-codex-cli/internal/projectdetect"
	"ollama-codex-cli/internal/session"
	"ollama-codex-cli/internal/tools"
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
	if err := modelprofile.Apply(modelProfile, &chatModel, &systemText, &toolProfile, &retries, func(name string) bool {
		return cmd.Flags().Changed(name)
	}); err != nil {
		return err
	}

	client := ollama.NewClient(chatHost, timeout)
	profile := tools.NormalizeProfile(toolProfile)
	sandbox := tools.NormalizeSandboxMode(sandboxMode)
	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("resolve workspace: %w", err)
	}
	project := projectdetect.Detect(workspaceAbs)
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
	checkpoints := checkpoint.New(workspaceAbs)
	sessionPath, err := filepath.Abs(sessionFile)
	if err != nil {
		return fmt.Errorf("resolve session-file: %w", err)
	}
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
	historyPath := filepath.Join(workspaceAbs, ".codew", "history.txt")

	s := session.New(chatModel, buildSystemPrompt(withProjectHint(systemText, project), toolsEnabled))
	if resumeSession {
		snap, loadErr := session.LoadFromFile(sessionPath)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to resume session: %v\n", loadErr)
		} else {
			s.Restore(snap)
			fmt.Printf("Resumed session from %s\n", sessionPath)
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

	fmt.Printf("Connected target: %s\n", chatHost)
	fmt.Printf("Project type: %s (%s)\n", project.Primary, strings.Join(project.All, ","))
	fmt.Printf("Model: %s\n", s.Model)
	if modelProfile != "" {
		fmt.Printf("Model profile: %s\n", modelProfile)
	}
	fmt.Printf("Tools: %t (auto-approve=%t)\n", toolsEnabled, autoApprove)
	fmt.Printf("Tool profile: %s\n", profile)
	fmt.Printf("Sandbox mode: %s\n", sandbox)
	fmt.Printf("Network escalation: allow=%t allow-tools=%v\n", networkAllow, networkAllowTool)
	fmt.Printf("MCP: %t (config=%s, tools=%d)\n", mcpEnabled, mcpConfig, len(mcpManager.Definitions()))
	fmt.Printf("Context limit: %d chars\n", maxContextChars)
	fmt.Printf("Auto context: %t (files=%d chars=%d)\n", autoContext, autoContextFiles, autoContextChars)
	fmt.Printf("Dry run: %t\n", dryRun)
	fmt.Printf("Auto checkpoint: %t\n", autoCheckpoint)
	fmt.Printf("Tool log: %t (%s)\n", toolLog, logPath)
	fmt.Printf("Trace log: %t (%s)\n", traceLog, tracePath)
	fmt.Printf("Retries: %d (backoff=%s, fallback=%s)\n", retries, retryBackoff, fallbackModel)
	fmt.Printf("Session file: %s (auto-save=%t)\n", sessionPath, autoSave)
	fmt.Println("Commands: /exit, /model <name>, /models, /system <text>, /reset, /save, /load, /checkpoint, /undo, /help")

	lineEditor := liner.NewLiner()
	defer lineEditor.Close()
	lineEditor.SetCtrlCAborts(true)
	if err := loadLineHistory(lineEditor, historyPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load history: %v\n", err)
	}
	defer func() {
		if err := saveLineHistory(lineEditor, historyPath); err != nil {
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
			done, cmdErr := runCommand(cmd.Context(), line, s, sessionPath, checkpoints, planner, client)
			if cmdErr != nil {
				fmt.Fprintf(os.Stderr, "command error: %v\n", cmdErr)
			} else if autoSave {
				if saveErr := saveSessionSnapshot(sessionPath, s); saveErr != nil {
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
			_ = turnLogger.Append(logging.TraceEvent{
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
			messages := withAutoContext(s.MessagesForModel(maxContextChars), autoCtx)
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			finishWorking := announceWorking("assistant is working")
			msg, usedModel, chatErr := chatWithRetry(ctx, client, s.Model, messages, toolDefs)
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
				_ = turnLogger.Append(logging.TraceEvent{
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
						_ = turnLogger.Append(logging.TraceEvent{
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
				if canOrchestrateInParallel(toolCalls, profile, sandbox, networkAllow, networkRules) {
					results := runToolCallsOrchestrated(executor, toolCalls, sandbox)
					for i, call := range toolCalls {
						toolResult := results[i]
						s.AddTool(call.Function.Name, call.ID, toolResult)
						fmt.Printf("\n[tool:%s] %s\n", call.Function.Name, summarizeToolResult(toolResult))
						writeToolLog(toolLogger, line, call.Function.Name, compactJSON(call.Function.Arguments), toolResult, true)
						if traceLog {
							_ = turnLogger.Append(logging.TraceEvent{
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
						IsMCP:        mcpManager != nil && mcpManager.HasTool(call.Function.Name),
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
						writeToolLog(toolLogger, line, call.Function.Name, compactJSON(call.Function.Arguments), toolResult, false)
						if traceLog {
							_ = turnLogger.Append(logging.TraceEvent{
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
						approved, allowAll := askToolApproval(lineEditor, call, preview)
						if allowAll {
							approveAll = true
						}
						if !approved {
							toolResult := `{"ok":false,"error":"tool call rejected by user"}`
							s.AddTool(call.Function.Name, call.ID, toolResult)
							fmt.Printf("\n[tool:%s] rejected\n", call.Function.Name)
							writeToolLog(toolLogger, line, call.Function.Name, compactJSON(call.Function.Arguments), toolResult, false)
							if traceLog {
								_ = turnLogger.Append(logging.TraceEvent{
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
						allowOnce, allowAlways := askNetworkEscalation(lineEditor, call.Function.Name)
						if allowAlways {
							networkRules[call.Function.Name] = true
						}
						if !allowOnce && !allowAlways {
							toolResult := `{"ok":false,"error":"network escalation denied by user"}`
							s.AddTool(call.Function.Name, call.ID, toolResult)
							fmt.Printf("[tool:%s] denied network escalation\n", call.Function.Name)
							writeToolLog(toolLogger, line, call.Function.Name, compactJSON(call.Function.Arguments), toolResult, false)
							if traceLog {
								_ = turnLogger.Append(logging.TraceEvent{
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
								_ = turnLogger.Append(logging.TraceEvent{
									Event:  "checkpoint_created",
									TurnID: turnID,
									Step:   step + 1,
									Tool:   call.Function.Name,
									Mode:   "chat",
								})
							}
						}
					}
					toolResult := executor.ExecuteWithSandbox(call, callSandbox)
					s.AddTool(call.Function.Name, call.ID, toolResult)
					fmt.Printf("[tool:%s] %s\n", call.Function.Name, summarizeToolResult(toolResult))
					writeToolLog(toolLogger, line, call.Function.Name, compactJSON(call.Function.Arguments), toolResult, true)
					if traceLog {
						_ = turnLogger.Append(logging.TraceEvent{
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
						fmt.Printf("[post-validate] %s\n", summarizeToolResult(validateResult))
						writeToolLog(toolLogger, line, "post_validate", strings.Join(postEditCmds, " && "), validateResult, true)
						if traceLog {
							_ = turnLogger.Append(logging.TraceEvent{
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
			_ = turnLogger.Append(logging.TraceEvent{
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
			if saveErr := saveSessionSnapshot(sessionPath, s); saveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: auto-save failed: %v\n", saveErr)
			}
		}
	}
}

func askToolApproval(lineEditor *liner.State, call ollama.ToolCall, preview string) (approved bool, allowAll bool) {
	args := compactJSON(call.Function.Arguments)
	if preview != "" {
		fmt.Printf("\n[tool:%s preview]\n%s\n", call.Function.Name, colorizeDiff(preview))
	}
	line, err := lineEditor.Prompt(fmt.Sprintf("approve tool %s args=%s ? [y/N/a]: ", call.Function.Name, args))
	if err != nil {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, false
	case "a", "all":
		return true, true
	default:
		return false, false
	}
}

func loadLineHistory(lineEditor *liner.State, path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()
	_, err = lineEditor.ReadHistory(file)
	return err
}

func saveLineHistory(lineEditor *liner.State, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = lineEditor.WriteHistory(file)
	return err
}

func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return string(raw)
	}
	return out.String()
}

func runCommand(ctx context.Context, line string, s *session.Session, sessionPath string, checkpoints *checkpoint.Manager, planner *plan.State, client *ollama.Client) (bool, error) {
	parts := strings.SplitN(line, " ", 2)
	name := parts[0]
	arg := ""
	if len(parts) == 2 {
		arg = strings.TrimSpace(parts[1])
	}

	switch name {
	case "/exit", "/quit":
		return true, nil
	case "/reset":
		s.Reset()
		fmt.Println("session reset")
		return false, nil
	case "/model":
		if arg == "" {
			return false, errors.New("usage: /model <name>")
		}
		s.Model = arg
		fmt.Printf("model changed to: %s\n", s.Model)
		return false, nil
	case "/models":
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		models, err := client.ListModels(reqCtx)
		if err != nil {
			return false, err
		}
		if len(models) == 0 {
			fmt.Println("no models found")
			return false, nil
		}
		fmt.Println("available models:")
		for _, m := range models {
			marker := " "
			if m.Name == s.Model {
				marker = "*"
			}
			fmt.Printf("%s %s\n", marker, m.Name)
		}
		return false, nil
	case "/system":
		if arg == "" {
			return false, errors.New("usage: /system <text>")
		}
		s.System = buildSystemPrompt(arg, toolsEnabled)
		s.Reset()
		fmt.Println("system prompt updated and history reset")
		return false, nil
	case "/help":
		fmt.Println("/exit | /quit  : end session")
		fmt.Println("/model <name>  : switch model")
		fmt.Println("/models        : list available models")
		fmt.Println("/system <text> : change system prompt and reset history")
		fmt.Println("/reset         : clear chat history")
		fmt.Println("/save          : save current session file")
		fmt.Println("/load          : load current session file")
		fmt.Println("/checkpoint    : create rollback checkpoint")
		fmt.Println("/undo          : restore latest checkpoint")
		fmt.Println("/plan <step>   : add plan item")
		fmt.Println("/plan-list     : show plan")
		fmt.Println("/plan-doing N  : mark item N in progress")
		fmt.Println("/plan-done N   : mark item N completed")
		return false, nil
	case "/save":
		if err := saveSessionSnapshot(sessionPath, s); err != nil {
			return false, err
		}
		fmt.Printf("session saved: %s\n", sessionPath)
		return false, nil
	case "/load":
		snap, err := session.LoadFromFile(sessionPath)
		if err != nil {
			return false, err
		}
		s.Restore(snap)
		fmt.Printf("session loaded: %s\n", sessionPath)
		return false, nil
	case "/checkpoint":
		id, err := checkpoints.Create()
		if err != nil {
			return false, err
		}
		fmt.Printf("checkpoint created: %s\n", id)
		return false, nil
	case "/undo":
		id, err := checkpoints.RestoreLatest()
		if err != nil {
			return false, err
		}
		fmt.Printf("restored checkpoint: %s\n", id)
		return false, nil
	case "/plan":
		if arg == "" {
			return false, errors.New("usage: /plan <step>")
		}
		planner.Add(arg)
		fmt.Println("plan item added")
		return false, nil
	case "/plan-list":
		fmt.Print(planner.Render())
		return false, nil
	case "/plan-doing":
		idx, err := parsePositiveInt(arg)
		if err != nil {
			return false, err
		}
		return false, planner.Set(idx, plan.InProgress)
	case "/plan-done":
		idx, err := parsePositiveInt(arg)
		if err != nil {
			return false, err
		}
		return false, planner.Set(idx, plan.Completed)
	default:
		return false, fmt.Errorf("unknown command: %s", name)
	}
}

func saveSessionSnapshot(path string, s *session.Session) error {
	return session.SaveToFile(path, s.Snapshot())
}

func parsePositiveInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("index is required")
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, errors.New("index must be positive integer")
	}
	return n, nil
}

func summarizeToolResult(raw string) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "done"
	}
	ok := asBool(obj["ok"])
	tool := asString(obj["tool"])
	if !ok {
		errMsg := asString(obj["error"])
		if errMsg == "" {
			errMsg = "failed"
		}
		return "ok=false error=" + errMsg
	}

	switch tool {
	case "shell_exec":
		return fmt.Sprintf("ok=true exit_error=%q timed_out=%t", asString(obj["exit_error"]), asBool(obj["timed_out"]))
	case "replace_in_file":
		return fmt.Sprintf("ok=true replaced=%d path=%s", asInt(obj["replaced"]), asString(obj["path"]))
	case "write_file":
		return fmt.Sprintf("ok=true bytes_written=%d path=%s", asInt(obj["bytes_written"]), asString(obj["path"]))
	case "apply_patch":
		return fmt.Sprintf("ok=true checked=%t applied=%t", asBool(obj["checked"]), asBool(obj["applied"]))
	case "list_files":
		return fmt.Sprintf("ok=true files=%d", asInt(obj["count"]))
	case "post_validate":
		return fmt.Sprintf("ok=%t commands=%d", ok, lenAny(obj["commands"]))
	default:
		return "ok=true"
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

func lenAny(v any) int {
	switch arr := v.(type) {
	case []any:
		return len(arr)
	default:
		return 0
	}
}

func toolCallSucceeded(raw string) bool {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return false
	}
	return asBool(obj["ok"])
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

func withAutoContext(messages []ollama.Message, autoCtx string) []ollama.Message {
	if strings.TrimSpace(autoCtx) == "" || len(messages) == 0 {
		return messages
	}
	last := messages[len(messages)-1]
	if last.Role != "user" {
		out := make([]ollama.Message, 0, len(messages)+1)
		out = append(out, messages...)
		out = append(out, ollama.Message{Role: "system", Content: autoCtx})
		return out
	}

	out := make([]ollama.Message, 0, len(messages)+1)
	out = append(out, messages[:len(messages)-1]...)
	out = append(out, ollama.Message{Role: "system", Content: autoCtx})
	out = append(out, last)
	return out
}

func canOrchestrateInParallel(calls []ollama.ToolCall, profile string, sandbox string, networkAllow bool, networkRules map[string]bool) bool {
	if len(calls) < 2 {
		return false
	}
	for _, c := range calls {
		if !isParallelSafeTool(c.Function.Name) {
			return false
		}
		if chatloop.NeedsNetworkEscalation(chatloop.ApprovalRequest{
			ToolName:     c.Function.Name,
			Sandbox:      sandbox,
			NetworkAllow: networkAllow,
			NetworkRules: networkRules,
			Profile:      profile,
		}) {
			return false
		}
	}
	return true
}

func isParallelSafeTool(name string) bool {
	switch name {
	case "read_file", "list_files", "web_search":
		return true
	default:
		return false
	}
}

func runToolCallsOrchestrated(executor *tools.Executor, calls []ollama.ToolCall, sandbox string) []string {
	results := make([]string, len(calls))
	done := make([]bool, len(calls))
	remaining := len(calls)

	for remaining > 0 {
		ready := []int{}
		for i, call := range calls {
			if done[i] {
				continue
			}
			if depsSatisfied(call, calls, done) {
				ready = append(ready, i)
			}
		}
		if len(ready) == 0 {
			for i := range calls {
				if !done[i] {
					results[i] = `{"ok":false,"error":"orchestration deadlock: unresolved dependencies"}`
					done[i] = true
					remaining--
				}
			}
			break
		}

		var wg sync.WaitGroup
		wg.Add(len(ready))
		for _, idx := range ready {
			idx := idx
			go func() {
				defer wg.Done()
				results[idx] = executor.ExecuteWithSandbox(calls[idx], sandbox)
			}()
		}
		wg.Wait()
		for _, idx := range ready {
			done[idx] = true
			remaining--
		}
	}
	return results
}

func depsSatisfied(call ollama.ToolCall, all []ollama.ToolCall, done []bool) bool {
	deps := callDependencies(call)
	if len(deps) == 0 {
		return true
	}
	for _, dep := range deps {
		found := false
		for i, c := range all {
			if done[i] && c.Function.Name == dep {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func callDependencies(call ollama.ToolCall) []string {
	var obj map[string]any
	if err := json.Unmarshal(call.Function.Arguments, &obj); err != nil {
		return nil
	}
	raw, ok := obj["_depends_on"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func askNetworkEscalation(lineEditor *liner.State, toolName string) (allowOnce bool, allowAlways bool) {
	line, err := lineEditor.Prompt(fmt.Sprintf("network escalation required for %s. allow once [y], always [a], deny [N]: ", toolName))
	if err != nil {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, false
	case "a", "all", "always":
		return false, true
	default:
		return false, false
	}
}

func colorizeDiff(text string) string {
	lines := strings.Split(text, "\n")
	var out strings.Builder
	for i, line := range lines {
		colored := line
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			colored = "\x1b[36m" + line + "\x1b[0m"
		case strings.HasPrefix(line, "@@"):
			colored = "\x1b[33m" + line + "\x1b[0m"
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			colored = "\x1b[32m" + line + "\x1b[0m"
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			colored = "\x1b[31m" + line + "\x1b[0m"
		}
		out.WriteString(colored)
		if i != len(lines)-1 {
			out.WriteString("\n")
		}
	}
	return out.String()
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
