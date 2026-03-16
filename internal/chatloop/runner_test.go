package chatloop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ollama-codex-cli/internal/checkpoint"
	"ollama-codex-cli/internal/ollama"
	"ollama-codex-cli/internal/session"
	"ollama-codex-cli/internal/tools"
)

type fakeChatClient struct {
	messages []ollama.Message
	index    int
}

func (f *fakeChatClient) Chat(ctx context.Context, model string, messages []ollama.Message, defs []ollama.ToolDefinition) (ollama.Message, error) {
	if f.index >= len(f.messages) {
		return ollama.Message{}, context.DeadlineExceeded
	}
	msg := f.messages[f.index]
	f.index++
	return msg, nil
}

type fakeExecutor struct {
	results []string
	index   int
}

func (f *fakeExecutor) ExecuteWithSandbox(call ollama.ToolCall, sandbox string) string {
	if f.index >= len(f.results) {
		return `{"ok":false,"error":"unexpected tool execution"}`
	}
	out := f.results[f.index]
	f.index++
	return out
}

func TestRunnerExecutesToolAndReturnsFinalAnswer(t *testing.T) {
	runner := Runner{
		Client:          &fakeChatClient{messages: []ollama.Message{{Content: `{"name":"read_file","arguments":{"path":"README.md"}}`}, {Content: "done"}}},
		Executor:        &fakeExecutor{results: []string{`{"ok":true,"tool":"read_file","content":"hi"}`}},
		Timeout:         time.Second,
		MaxToolSteps:    3,
		MaxContextChars: 4000,
	}
	s := session.New("test-model", "system")
	s.AddUser("check")

	result, err := runner.Execute(context.Background(), RunRequest{
		Mode:         "run",
		Prompt:       "check",
		Session:      s,
		Allowed:      map[string]struct{}{"read_file": {}},
		ToolsEnabled: true,
		Profile:      tools.ProfileReadOnly,
		Sandbox:      tools.SandboxReadOnly,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Answer != "done" {
		t.Fatalf("expected final answer, got %q", result.Answer)
	}
	msgs := s.Messages()
	if len(msgs) < 4 || msgs[len(msgs)-1].Content != "done" {
		t.Fatalf("expected final assistant response in session: %#v", msgs)
	}
}

func TestRunnerStoresDeniedToolResult(t *testing.T) {
	runner := Runner{
		Client:          &fakeChatClient{messages: []ollama.Message{{Content: `{"name":"shell_exec","arguments":{"command":"rm -rf ."}}`}, {Content: "stopped"}}},
		Executor:        &fakeExecutor{},
		Timeout:         time.Second,
		MaxToolSteps:    3,
		MaxContextChars: 4000,
	}
	s := session.New("test-model", "system")
	s.AddUser("check")

	_, err := runner.Execute(context.Background(), RunRequest{
		Mode:         "run",
		Prompt:       "check",
		Session:      s,
		Allowed:      map[string]struct{}{"shell_exec": {}},
		ToolsEnabled: true,
		Profile:      tools.ProfileWorkspaceWrite,
		Sandbox:      tools.SandboxWorkspaceWrite,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	msgs := s.Messages()
	found := false
	for _, msg := range msgs {
		if msg.Role == "tool" && msg.Name == "shell_exec" && msg.Content == `{"ok":false,"error":"tool call denied by policy"}` {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected denied tool result in session: %#v", msgs)
	}
}

func TestRunnerCreatesCheckpointBeforeMutatingTool(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "a.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	exec, err := tools.NewExecutor(ws, tools.ProfileWorkspaceWrite, false, tools.SandboxWorkspaceWrite, nil)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	runner := Runner{
		Client: &fakeChatClient{messages: []ollama.Message{
			{Content: `{"name":"write_file","arguments":{"path":"a.txt","content":"new\n"}}`},
			{Content: "done"},
		}},
		Executor:        exec,
		Checkpoints:     checkpoint.New(ws),
		Timeout:         time.Second,
		MaxToolSteps:    3,
		MaxContextChars: 4000,
		AutoCheckpoint:  true,
	}
	s := session.New("test-model", "system")
	s.AddUser("edit")

	if _, err := runner.Execute(context.Background(), RunRequest{
		Mode:         "run",
		Prompt:       "edit",
		Session:      s,
		Allowed:      map[string]struct{}{"write_file": {}},
		ToolsEnabled: true,
		Profile:      tools.ProfileWorkspaceWrite,
		Sandbox:      tools.SandboxWorkspaceWrite,
		Workspace:    ws,
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	indexPath := filepath.Join(ws, ".codew", "checkpoints", "index.json")
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("expected checkpoint index: %v", err)
	}
}

func TestRunnerRunsPostValidateAfterMutatingTool(t *testing.T) {
	ws := t.TempDir()
	exec, err := tools.NewExecutor(ws, tools.ProfileWorkspaceWrite, false, tools.SandboxWorkspaceWrite, nil)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	runner := Runner{
		Client: &fakeChatClient{messages: []ollama.Message{
			{Content: `{"name":"write_file","arguments":{"path":"a.txt","content":"new\n"}}`},
			{Content: "done"},
		}},
		Executor:        exec,
		Timeout:         time.Second,
		MaxToolSteps:    3,
		MaxContextChars: 4000,
		AutoValidate:    true,
		PostEditCmds:    []string{"printf ok"},
	}
	s := session.New("test-model", "system")
	s.AddUser("edit")

	if _, err := runner.Execute(context.Background(), RunRequest{
		Mode:         "run",
		Prompt:       "edit",
		Session:      s,
		Allowed:      map[string]struct{}{"write_file": {}},
		ToolsEnabled: true,
		Profile:      tools.ProfileWorkspaceWrite,
		Sandbox:      tools.SandboxWorkspaceWrite,
		Workspace:    ws,
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	msgs := s.Messages()
	found := false
	for _, msg := range msgs {
		if msg.Role == "tool" && msg.Name == "post_validate" {
			var payload map[string]any
			if err := json.Unmarshal([]byte(msg.Content), &payload); err != nil {
				t.Fatalf("unmarshal validation result: %v", err)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected post_validate result in session")
	}
}
