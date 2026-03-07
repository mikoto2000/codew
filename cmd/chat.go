package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ollama-codex-cli/internal/agent"
	"ollama-codex-cli/internal/ollama"
	"ollama-codex-cli/internal/session"
	"ollama-codex-cli/internal/tools"
)

const toolPromptSuffix = `
If you need to use a tool, respond with JSON only (no markdown), using one of these formats:
{"name":"tool_name","arguments":{...}}
{"tool_calls":[{"name":"tool_name","arguments":{...}}]}
For file edits, prefer apply_patch over full-file overwrite when possible.
After receiving tool results, provide a normal final answer for the user.
`

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session",
	RunE:  runChat,
}

func runChat(cmd *cobra.Command, _ []string) error {
	client := ollama.NewClient(chatHost, timeout)
	profile := tools.NormalizeProfile(toolProfile)
	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("resolve workspace: %w", err)
	}
	executor, err := tools.NewExecutor(workspaceAbs, profile)
	if err != nil {
		return err
	}
	sessionPath, err := filepath.Abs(sessionFile)
	if err != nil {
		return fmt.Errorf("resolve session-file: %w", err)
	}

	s := session.New(chatModel, buildSystemPrompt(systemText, toolsEnabled))
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
		allowed = tools.AllowedToolNamesForProfile(profile)
	}

	fmt.Printf("Connected target: %s\n", chatHost)
	fmt.Printf("Model: %s\n", s.Model)
	fmt.Printf("Tools: %t (auto-approve=%t)\n", toolsEnabled, autoApprove)
	fmt.Printf("Tool profile: %s\n", profile)
	fmt.Printf("Context limit: %d chars\n", maxContextChars)
	fmt.Printf("Retries: %d (backoff=%s, fallback=%s)\n", retries, retryBackoff, fallbackModel)
	fmt.Printf("Session file: %s (auto-save=%t)\n", sessionPath, autoSave)
	fmt.Println("Commands: /exit, /model <name>, /system <text>, /reset, /save, /load, /help")

	reader := bufio.NewReader(os.Stdin)
	approveAll := autoApprove

	for {
		fmt.Print("you> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && len(strings.TrimSpace(line)) > 0 {
				// Continue processing the final line without a trailing newline.
			} else if errors.Is(err, io.EOF) {
				return nil
			} else if len(line) == 0 {
				return nil
			}
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			done, cmdErr := runCommand(line, s, sessionPath)
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
		fmt.Print("assistant> ")

		finalPrinted := false
		for step := 0; step < maxToolSteps; step++ {
			messages := s.MessagesForModel(maxContextChars)
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			msg, usedModel, chatErr := chatWithRetry(ctx, client, s.Model, messages, toolDefs)
			cancel()

			if chatErr != nil {
				fmt.Fprintf(os.Stderr, "\nrequest failed: %v\n", chatErr)
				if step == 0 {
					s.RollbackLastUser()
				}
				if usedModel != s.Model {
					fmt.Printf("\n[model fallback] using %s\n", usedModel)
				}
				break
			}

			toolCalls, parsed := agent.ExtractToolCalls(msg, allowed)
			if toolsEnabled && len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
				s.AddAssistantMessage(msg)

				for _, call := range toolCalls {
					if !approveAll {
						preview := ""
						if tools.IsMutatingTool(call.Function.Name) {
							preview = tools.Preview(call)
						}
						approved, allowAll := askToolApproval(reader, call, preview)
						if allowAll {
							approveAll = true
						}
						if !approved {
							toolResult := `{"ok":false,"error":"tool call rejected by user"}`
							s.AddTool(call.Function.Name, call.ID, toolResult)
							fmt.Printf("\n[tool:%s] rejected\n", call.Function.Name)
							continue
						}
					}

					fmt.Printf("\n[tool:%s] running...\n", call.Function.Name)
					toolResult := executor.Execute(call)
					s.AddTool(call.Function.Name, call.ID, toolResult)
					fmt.Printf("[tool:%s] %s\n", call.Function.Name, summarizeToolResult(toolResult))
					if autoValidate && tools.IsMutatingTool(call.Function.Name) && toolCallSucceeded(toolResult) {
						validateResult := runValidation(workspaceAbs, postEditCmds)
						s.AddTool("post_validate", "", validateResult)
						fmt.Printf("[post-validate] %s\n", summarizeToolResult(validateResult))
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
				fmt.Print(answer)
				finalPrinted = true
			}
			break
		}

		if !finalPrinted {
			fmt.Print("(no response)")
		}
		fmt.Println()
		if autoSave {
			if saveErr := saveSessionSnapshot(sessionPath, s); saveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: auto-save failed: %v\n", saveErr)
			}
		}
	}
}

func askToolApproval(reader *bufio.Reader, call ollama.ToolCall, preview string) (approved bool, allowAll bool) {
	args := compactJSON(call.Function.Arguments)
	if preview != "" {
		fmt.Printf("\n[tool:%s preview]\n%s\n", call.Function.Name, preview)
	}
	fmt.Printf("approve tool %s args=%s ? [y/N/a]: ", call.Function.Name, args)
	line, _ := reader.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, false
	case "a", "all":
		return true, true
	default:
		return false, false
	}
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

func runCommand(line string, s *session.Session, sessionPath string) (bool, error) {
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
		fmt.Println("/system <text> : change system prompt and reset history")
		fmt.Println("/reset         : clear chat history")
		fmt.Println("/save          : save current session file")
		fmt.Println("/load          : load current session file")
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
	default:
		return false, fmt.Errorf("unknown command: %s", name)
	}
}

func saveSessionSnapshot(path string, s *session.Session) error {
	return session.SaveToFile(path, s.Snapshot())
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
