package chatloop

import (
	"encoding/json"
	"strings"
	"sync"

	"ollama-codex-cli/internal/ollama"
	"ollama-codex-cli/internal/tools"
)

func WithAutoContext(messages []ollama.Message, autoCtx string) []ollama.Message {
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

func CanOrchestrateInParallel(calls []ollama.ToolCall, profile string, sandbox string, networkAllow bool, networkRules map[string]bool) bool {
	if len(calls) < 2 {
		return false
	}
	for _, c := range calls {
		if !isParallelSafeTool(c.Function.Name) {
			return false
		}
		if NeedsNetworkEscalation(ApprovalRequest{
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

func RunToolCallsOrchestrated(executor *tools.Executor, calls []ollama.ToolCall, sandbox string) []string {
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

func isParallelSafeTool(name string) bool {
	switch name {
	case "read_file", "list_files", "web_search":
		return true
	default:
		return false
	}
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
