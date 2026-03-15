package tools

import "testing"

func TestCheckShellCommandAllowed(t *testing.T) {
	if err := CheckShellCommandAllowed(ProfileFull, "git status --short"); err != nil {
		t.Fatalf("expected git status to be allowed: %v", err)
	}
	if err := CheckShellCommandAllowed(ProfileFull, "rm -rf ."); err == nil {
		t.Fatalf("expected rm to be rejected")
	}
}
