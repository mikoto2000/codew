package tools

import "fmt"

type Permission uint8

const (
	PermRead Permission = 1 << iota
	PermWrite
	PermNetwork
	PermExec
)

const (
	SandboxReadOnly       = "read-only"
	SandboxWorkspaceWrite = "workspace-write"
	SandboxFull           = "full"
)

func NormalizeSandboxMode(mode string) string {
	switch mode {
	case SandboxReadOnly, SandboxFull:
		return mode
	default:
		return SandboxWorkspaceWrite
	}
}

func AllowedPermissions(mode string) Permission {
	switch NormalizeSandboxMode(mode) {
	case SandboxReadOnly:
		return PermRead
	case SandboxWorkspaceWrite:
		return PermRead | PermWrite
	case SandboxFull:
		return PermRead | PermWrite | PermNetwork | PermExec
	default:
		return PermRead | PermWrite
	}
}

func RequiredPermissions(toolName string, isMCP bool) Permission {
	if isMCP {
		return PermNetwork | PermExec
	}
	switch toolName {
	case "list_files", "read_file":
		return PermRead
	case "write_file", "replace_in_file", "apply_patch":
		return PermWrite
	case "web_search":
		return PermNetwork
	case "shell_exec":
		return PermExec
	default:
		return 0
	}
}

func CheckPermissions(mode string, required Permission) error {
	allowed := AllowedPermissions(mode)
	if required&allowed != required {
		return fmt.Errorf("permission denied: required=%s allowed=%s", permissionsString(required), permissionsString(allowed))
	}
	return nil
}

func RequiresNetwork(toolName string, isMCP bool) bool {
	return RequiredPermissions(toolName, isMCP)&PermNetwork != 0
}

func AllowsNetwork(mode string) bool {
	return AllowedPermissions(mode)&PermNetwork != 0
}

func permissionsString(p Permission) string {
	if p == 0 {
		return "none"
	}
	parts := []string{}
	if p&PermRead != 0 {
		parts = append(parts, "read")
	}
	if p&PermWrite != 0 {
		parts = append(parts, "write")
	}
	if p&PermNetwork != 0 {
		parts = append(parts, "network")
	}
	if p&PermExec != 0 {
		parts = append(parts, "exec")
	}
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += ","
		}
		out += part
	}
	return out
}
