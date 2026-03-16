package chatloop

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"ollama-codex-cli/internal/agent"
	"ollama-codex-cli/internal/logging"
	"ollama-codex-cli/internal/ollama"
	"ollama-codex-cli/internal/session"
	"ollama-codex-cli/internal/tools"
)

type ChatClient interface {
	Chat(ctx context.Context, model string, messages []ollama.Message, tools []ollama.ToolDefinition) (ollama.Message, error)
}

type ToolExecutor interface {
	ExecuteWithSandbox(call ollama.ToolCall, sandbox string) string
}

type CheckpointManager interface {
	Create() (string, error)
}

type Runner struct {
	Client          ChatClient
	Executor        ToolExecutor
	Checkpoints     CheckpointManager
	ToolLogger      *logging.ToolLogger
	TraceLogger     *logging.TurnLogger
	Timeout         time.Duration
	Retries         int
	RetryBackoff    time.Duration
	FallbackModel   string
	MaxToolSteps    int
	MaxContextChars int
	AutoCheckpoint  bool
	DryRun          bool
	AutoValidate    bool
	PostEditCmds    []string
	ToolLogEnabled  bool
	TraceLogEnabled bool
}

type RunRequest struct {
	Mode         string
	Prompt       string
	Session      *session.Session
	ToolDefs     []ollama.ToolDefinition
	Allowed      map[string]struct{}
	ToolsEnabled bool
	Profile      string
	Sandbox      string
	NetworkAllow bool
	NetworkRules map[string]bool
	Workspace    string
	AutoContext  string
}

type RunResult struct {
	Answer    string
	ToolCalls int
}

func (r *Runner) Execute(ctx context.Context, req RunRequest) (RunResult, error) {
	turnStart := time.Now()
	turnID := fmt.Sprintf("%s-%d", req.Mode, turnStart.UnixNano())
	toolCalls := 0
	r.appendTrace(logging.TraceEvent{
		Event:  "turn_started",
		TurnID: turnID,
		Mode:   req.Mode,
		Input:  req.Prompt,
		Model:  req.Session.Model,
	})

	checkpointed := false
	for step := 0; step < r.MaxToolSteps; step++ {
		messages := WithAutoContext(req.Session.MessagesForModel(r.MaxContextChars), req.AutoContext)
		msg, usedModel, err := r.chatWithRetry(ctx, req.Session.Model, messages, req.ToolDefs)
		if err != nil {
			return RunResult{}, err
		}
		r.appendTrace(logging.TraceEvent{
			Event:  "model_response_received",
			TurnID: turnID,
			Step:   step + 1,
			Mode:   req.Mode,
			Model:  usedModel,
		})

		parseResult := agent.ExtractToolCalls(msg, req.Allowed)
		parsedCalls := parseResult.Calls
		if req.ToolsEnabled && len(parsedCalls) > 0 {
			req.Session.AddAssistantMessage(msg)
			for _, call := range parsedCalls {
				r.appendTrace(logging.TraceEvent{
					Event:      "tool_call_parsed",
					TurnID:     turnID,
					Step:       step + 1,
					ToolCallID: call.ID,
					Tool:       call.Function.Name,
					Mode:       req.Mode,
				})
			}

			results := map[int]string{}
			if CanOrchestrateInParallel(parsedCalls, req.Profile, req.Sandbox, req.NetworkAllow, req.NetworkRules) {
				parallel := RunToolCallsOrchestrated(r.Executor, parsedCalls, req.Sandbox)
				for i, result := range parallel {
					results[i] = result
				}
			} else {
				for i, call := range parsedCalls {
					callSandbox := req.Sandbox
					decision := Decide(ApprovalRequest{
						ToolName:     call.Function.Name,
						IsMutating:   tools.IsMutatingTool(call.Function.Name),
						Sandbox:      req.Sandbox,
						AutoApprove:  true,
						NetworkAllow: req.NetworkAllow,
						NetworkRules: req.NetworkRules,
						Profile:      req.Profile,
					})
					if decision == DecisionDenied {
						results[i] = `{"ok":false,"error":"tool call denied by policy"}`
						r.appendTrace(logging.TraceEvent{
							Event:      "tool_call_denied",
							TurnID:     turnID,
							Step:       step + 1,
							ToolCallID: call.ID,
							Tool:       call.Function.Name,
							Mode:       req.Mode,
							Error:      "tool call denied by policy",
						})
						continue
					}
					if decision == DecisionNeedsNetworkEscalation && (req.NetworkAllow || req.NetworkRules[call.Function.Name]) {
						callSandbox = tools.SandboxFull
					}
					if r.AutoCheckpoint && !checkpointed && tools.IsMutatingTool(call.Function.Name) && !r.DryRun && r.Checkpoints != nil {
						if _, err := r.Checkpoints.Create(); err == nil {
							checkpointed = true
							r.appendTrace(logging.TraceEvent{
								Event:  "checkpoint_created",
								TurnID: turnID,
								Step:   step + 1,
								Tool:   call.Function.Name,
								Mode:   req.Mode,
							})
						}
					}
					results[i] = r.Executor.ExecuteWithSandbox(call, callSandbox)
				}
			}

			for i, call := range parsedCalls {
				res := results[i]
				req.Session.AddTool(call.Function.Name, call.ID, res)
				r.writeToolLog(req.Prompt, call.Function.Name, CompactJSON(call.Function.Arguments), res, true)
				r.appendTrace(logging.TraceEvent{
					Event:      "tool_call_executed",
					TurnID:     turnID,
					Step:       step + 1,
					ToolCallID: call.ID,
					Tool:       call.Function.Name,
					Mode:       req.Mode,
				})
				toolCalls++
				if r.AutoValidate && tools.IsMutatingTool(call.Function.Name) && toolCallSucceeded(res) {
					validateResult := runValidation(req.Workspace, r.Timeout, r.PostEditCmds)
					req.Session.AddTool("post_validate", "", validateResult)
					r.writeToolLog(req.Prompt, "post_validate", strings.Join(r.PostEditCmds, " && "), validateResult, true)
					r.appendTrace(logging.TraceEvent{
						Event:  "post_validate_finished",
						TurnID: turnID,
						Step:   step + 1,
						Tool:   call.Function.Name,
						Mode:   req.Mode,
					})
					toolCalls++
				}
			}
			continue
		}

		answer := strings.TrimSpace(msg.Content)
		req.Session.AddAssistantMessage(msg)
		r.appendTrace(logging.TraceEvent{
			Event:      "turn_finished",
			TurnID:     turnID,
			Mode:       req.Mode,
			Input:      req.Prompt,
			DurationMS: time.Since(turnStart).Milliseconds(),
			ToolCalls:  toolCalls,
		})
		return RunResult{Answer: answer, ToolCalls: toolCalls}, nil
	}
	return RunResult{}, fmt.Errorf("max-tool-steps reached without final response")
}

func (r *Runner) chatWithRetry(ctx context.Context, primaryModel string, messages []ollama.Message, defs []ollama.ToolDefinition) (ollama.Message, string, error) {
	models := []string{primaryModel}
	if strings.TrimSpace(r.FallbackModel) != "" && r.FallbackModel != primaryModel {
		models = append(models, r.FallbackModel)
	}

	var lastErr error
	for modelIndex, model := range models {
		for attempt := 0; attempt <= r.Retries; attempt++ {
			reqCtx, cancel := context.WithTimeout(ctx, r.Timeout)
			msg, err := r.Client.Chat(reqCtx, model, messages, defs)
			cancel()
			if err == nil {
				return msg, model, nil
			}
			lastErr = err
			if attempt < r.Retries {
				sleep := r.RetryBackoff * time.Duration(1<<attempt)
				select {
				case <-time.After(sleep):
				case <-ctx.Done():
					return ollama.Message{}, model, ctx.Err()
				}
			}
		}
		if modelIndex < len(models)-1 {
			continue
		}
	}
	return ollama.Message{}, primaryModel, lastErr
}

func (r *Runner) writeToolLog(turnInput, toolName, args, result string, approved bool) {
	if !r.ToolLogEnabled || r.ToolLogger == nil {
		return
	}
	_ = r.ToolLogger.Append(logging.ToolEvent{
		TurnInput: turnInput,
		Tool:      toolName,
		Args:      args,
		Result:    result,
		Approved:  approved,
	})
}

func (r *Runner) appendTrace(event logging.TraceEvent) {
	if !r.TraceLogEnabled || r.TraceLogger == nil {
		return
	}
	_ = r.TraceLogger.Append(event)
}

func toolCallSucceeded(raw string) bool {
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return false
	}
	ok, _ := obj["ok"].(bool)
	return ok
}

func runValidation(workspace string, timeout time.Duration, commands []string) string {
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
