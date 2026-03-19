package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"
)

type shellArgs struct {
	Command    string `json:"command"`
	Workdir    string `json:"workdir"`
	TimeoutSec int    `json:"timeout_sec"`
	PTY        bool   `json:"pty"`
}

func ShellCommand(raw json.RawMessage) (string, error) {
	var in shellArgs
	if err := decodeArgs(raw, &in); err != nil {
		return "", err
	}
	if strings.TrimSpace(in.Command) == "" {
		return "", errors.New("command is required")
	}
	return strings.TrimSpace(in.Command), nil
}

func (e *Executor) shellExec(raw json.RawMessage) (map[string]any, error) {
	command, err := ShellCommand(raw)
	if err != nil {
		return nil, err
	}
	if err := e.CheckShellCommandAllowed(command); err != nil {
		return nil, err
	}
	var in shellArgs
	if err := decodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if in.TimeoutSec <= 0 {
		in.TimeoutSec = 30
	}

	dir := e.workspace
	if strings.TrimSpace(in.Workdir) != "" {
		resolved, err := e.resolvePath(in.Workdir)
		if err != nil {
			return nil, err
		}
		dir = resolved
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(in.TimeoutSec)*time.Second)
	defer cancel()

	var c *exec.Cmd
	if in.PTY {
		c = exec.CommandContext(ctx, "script", "-qec", in.Command, "/dev/null")
	} else {
		c = exec.CommandContext(ctx, "bash", "-lc", in.Command)
	}
	c.Dir = dir
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err = c.Run()

	out := map[string]any{
		"workdir": dir,
		"command": in.Command,
		"pty":     in.PTY,
		"stdout":  truncate(stdout.String()),
		"stderr":  truncate(stderr.String()),
	}
	if err != nil {
		out["exit_error"] = err.Error()
	}
	if ctx.Err() == context.DeadlineExceeded {
		out["timed_out"] = true
	}
	return out, nil
}
