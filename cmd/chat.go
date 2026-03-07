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
	"strings"

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
	executor, err := tools.NewExecutor(workspaceRoot)
	if err != nil {
		return err
	}

	s := session.New(chatModel, buildSystemPrompt(systemText, toolsEnabled))
	toolDefs := []ollama.ToolDefinition(nil)
	allowed := map[string]struct{}{}
	if toolsEnabled {
		toolDefs = tools.Definitions()
		allowed = tools.AllowedToolNames()
	}

	fmt.Printf("Connected target: %s\n", chatHost)
	fmt.Printf("Model: %s\n", s.Model)
	fmt.Printf("Tools: %t (auto-approve=%t)\n", toolsEnabled, autoApprove)
	fmt.Println("Commands: /exit, /model <name>, /system <text>, /reset, /help")

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
			done, cmdErr := runCommand(line, s)
			if cmdErr != nil {
				fmt.Fprintf(os.Stderr, "command error: %v\n", cmdErr)
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
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			msg, chatErr := client.Chat(ctx, s.Model, s.Messages(), toolDefs)
			cancel()

			if chatErr != nil {
				fmt.Fprintf(os.Stderr, "\nrequest failed: %v\n", chatErr)
				if step == 0 {
					s.RollbackLastUser()
				}
				break
			}

			toolCalls, parsed := agent.ExtractToolCalls(msg, allowed)
			if toolsEnabled && len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
				s.AddAssistantMessage(msg)

				for _, call := range toolCalls {
					if !approveAll {
						approved, allowAll := askToolApproval(reader, call)
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
					fmt.Printf("[tool:%s] done\n", call.Function.Name)
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
	}
}

func askToolApproval(reader *bufio.Reader, call ollama.ToolCall) (approved bool, allowAll bool) {
	args := compactJSON(call.Function.Arguments)
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

func runCommand(line string, s *session.Session) (bool, error) {
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
		return false, nil
	default:
		return false, fmt.Errorf("unknown command: %s", name)
	}
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
