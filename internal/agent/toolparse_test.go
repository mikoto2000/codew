package agent

import (
	"testing"

	"ollama-codex-cli/internal/ollama"
)

func TestExtractToolCallsDirect(t *testing.T) {
	result := ExtractToolCalls(ollama.Message{
		Content: `{"name":"read_file","arguments":{"path":"README.md"}}`,
	}, map[string]struct{}{"read_file": {}})

	if !result.Parsed {
		t.Fatalf("expected parsed=true")
	}
	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %#v", result.Diagnostics)
	}
}

func TestExtractToolCallsMalformedJSON(t *testing.T) {
	result := ExtractToolCalls(ollama.Message{
		Content: `{"name":"read_file","arguments":{"path":"README.md"}`,
	}, map[string]struct{}{"read_file": {}})

	if !result.Parsed {
		t.Fatalf("expected parsed=true for malformed json")
	}
	if len(result.Calls) != 0 {
		t.Fatalf("expected no calls, got %d", len(result.Calls))
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "malformed_json" {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
}

func TestExtractToolCallsRejectsDisallowedTool(t *testing.T) {
	result := ExtractToolCalls(ollama.Message{
		Content: `{"name":"shell_exec","arguments":{"command":"pwd"}}`,
	}, map[string]struct{}{"read_file": {}})

	if !result.Parsed {
		t.Fatalf("expected parsed=true")
	}
	if len(result.Calls) != 0 {
		t.Fatalf("expected no calls, got %d", len(result.Calls))
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "rejected_by_allowlist" {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
}

func TestExtractToolCallsRejectsInvalidArguments(t *testing.T) {
	result := ExtractToolCalls(ollama.Message{
		Content: `{"name":"read_file","arguments":"README.md"}`,
	}, map[string]struct{}{"read_file": {}})

	if !result.Parsed {
		t.Fatalf("expected parsed=true")
	}
	if len(result.Calls) != 0 {
		t.Fatalf("expected no calls, got %d", len(result.Calls))
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "invalid_arguments" {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
}

func TestExtractToolCallsFromCodeBlock(t *testing.T) {
	result := ExtractToolCalls(ollama.Message{
		Content: "```json\n{\"name\":\"read_file\",\"arguments\":{\"path\":\"README.md\"}}\n```",
	}, map[string]struct{}{"read_file": {}})

	if !result.Parsed {
		t.Fatalf("expected parsed=true")
	}
	if len(result.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(result.Calls))
	}
}
