package tools

import "testing"

func TestCheckShellCommandAllowed(t *testing.T) {
	if err := CheckShellCommandAllowed(ProfileFull, "git status --short", nil); err != nil {
		t.Fatalf("expected git status to be allowed: %v", err)
	}
	if err := CheckShellCommandAllowed(ProfileFull, "make test", nil); err != nil {
		t.Fatalf("expected make test to be allowed: %v", err)
	}
	if err := CheckShellCommandAllowed(ProfileFull, "rm -rf .", nil); err == nil {
		t.Fatalf("expected rm to be rejected")
	}
}

func TestCheckShellCommandAllowedWithUserAllowlist(t *testing.T) {
	if err := CheckShellCommandAllowed(ProfileFull, "terraform plan", []string{"terraform plan"}); err != nil {
		t.Fatalf("expected terraform plan to be allowed: %v", err)
	}
	if err := CheckShellCommandAllowed(ProfileFull, "terraform apply", []string{"terraform plan"}); err == nil {
		t.Fatalf("expected terraform apply to be rejected")
	}
}
