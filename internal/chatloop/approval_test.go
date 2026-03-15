package chatloop

import (
	"testing"

	"ollama-codex-cli/internal/tools"
)

func TestDecideAllowedForReadOnlyTool(t *testing.T) {
	decision := Decide(ApprovalRequest{
		ToolName: "read_file",
		Profile:  tools.ProfileReadOnly,
		Sandbox:  tools.SandboxReadOnly,
	})
	if decision != DecisionAllowed {
		t.Fatalf("expected allowed, got %s", decision)
	}
}

func TestDecideNeedsUserApprovalForMutatingTool(t *testing.T) {
	decision := Decide(ApprovalRequest{
		ToolName:   "apply_patch",
		Profile:    tools.ProfileWorkspaceWrite,
		Sandbox:    tools.SandboxWorkspaceWrite,
		IsMutating: true,
	})
	if decision != DecisionNeedsUserApproval {
		t.Fatalf("expected needs-user-approval, got %s", decision)
	}
}

func TestDecideNeedsNetworkEscalation(t *testing.T) {
	decision := Decide(ApprovalRequest{
		ToolName: "web_search",
		Profile:  tools.ProfileFull,
		Sandbox:  tools.SandboxWorkspaceWrite,
	})
	if decision != DecisionNeedsNetworkEscalation {
		t.Fatalf("expected needs-network-escalation, got %s", decision)
	}
}

func TestDecideDeniedForDisallowedTool(t *testing.T) {
	decision := Decide(ApprovalRequest{
		ToolName: "shell_exec",
		Profile:  tools.ProfileWorkspaceWrite,
		Sandbox:  tools.SandboxWorkspaceWrite,
	})
	if decision != DecisionDenied {
		t.Fatalf("expected denied, got %s", decision)
	}
}
