package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"ollama-codex-cli/internal/agent"
	"ollama-codex-cli/internal/checkpoint"
	"ollama-codex-cli/internal/contextloader"
	"ollama-codex-cli/internal/logging"
	"ollama-codex-cli/internal/modelprofile"
	"ollama-codex-cli/internal/ollama"
	"ollama-codex-cli/internal/session"
	"ollama-codex-cli/internal/tools"
)

var runCmd = &cobra.Command{
	Use:   "run <prompt>",
	Short: "Run a single non-interactive prompt",
	Args:  cobra.ExactArgs(1),
	RunE:  runOnce,
}

func runOnce(cmd *cobra.Command, args []string) error {
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
	profile := tools.NormalizeProfile(toolProfile)
	client := ollama.NewClient(chatHost, timeout)
	executor, err := tools.NewExecutor(workspaceAbs, profile, dryRun)
	if err != nil {
		return err
	}
	cp := checkpoint.New(workspaceAbs)

	logPath := toolLogFile
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(workspaceAbs, logPath)
	}
	toolLogger := logging.NewToolLogger(logPath)

	s := session.New(chatModel, buildSystemPrompt(systemText, toolsEnabled))
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
		allowed = tools.AllowedToolNamesForProfile(profile)
	}

	checkpointed := false
	for step := 0; step < maxToolSteps; step++ {
		messages := withAutoContext(s.MessagesForModel(maxContextChars), autoCtx)
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		msg, _, chatErr := chatWithRetry(ctx, client, s.Model, messages, toolDefs)
		cancel()
		if chatErr != nil {
			return chatErr
		}

		toolCalls, _ := agent.ExtractToolCalls(msg, allowed)
		if toolsEnabled && len(toolCalls) > 0 {
			s.AddAssistantMessage(msg)
			results := map[int]string{}
			if canRunInParallel(toolCalls) {
				parallel := runToolCallsParallel(executor, toolCalls)
				for i, result := range parallel {
					results[i] = result
				}
			} else {
				for i, call := range toolCalls {
					if autoCheckpoint && !checkpointed && tools.IsMutatingTool(call.Function.Name) && !dryRun {
						if id, e := cp.Create(); e == nil {
							checkpointed = true
							fmt.Fprintf(os.Stderr, "[checkpoint] created: %s\n", id)
						}
					}
					results[i] = executor.Execute(call)
				}
			}

			for i, call := range toolCalls {
				res := results[i]
				s.AddTool(call.Function.Name, call.ID, res)
				writeToolLog(toolLogger, prompt, call.Function.Name, compactJSON(call.Function.Arguments), res, true)
				if autoValidate && tools.IsMutatingTool(call.Function.Name) && toolCallSucceeded(res) {
					validateResult := runValidation(workspaceAbs, postEditCmds)
					s.AddTool("post_validate", "", validateResult)
					writeToolLog(toolLogger, prompt, "post_validate", strings.Join(postEditCmds, " && "), validateResult, true)
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
