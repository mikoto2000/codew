package chatloop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mikoto2000/codew/internal/checkpoint"
	"github.com/mikoto2000/codew/internal/ollama"
	"github.com/mikoto2000/codew/internal/plan"
	"github.com/mikoto2000/codew/internal/session"
)

type CommandOptions struct {
	SessionPath       string
	ChatHost          string
	ToolsEnabled      bool
	Timeout           time.Duration
	BuildSystemPrompt func(string, bool) string
}

func ExecuteCommand(ctx context.Context, line string, s *session.Session, checkpoints *checkpoint.Manager, planner *plan.State, client *ollama.Client, opts CommandOptions) (bool, error) {
	parts := strings.SplitN(line, " ", 2)
	name := parts[0]
	arg := ""
	if len(parts) == 2 {
		arg = strings.TrimSpace(parts[1])
	}

	switch name {
	case "/exit", "/quit":
		return true, nil
	case "/reset":
		s.Reset()
		fmt.Println("session reset")
		return false, nil
	case "/model":
		if arg == "" {
			return false, errors.New("usage: /model <name>")
		}
		s.Model = arg
		fmt.Printf("model changed to: %s\n", s.Model)
		return false, nil
	case "/models":
		reqCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
		models, err := client.ListModels(reqCtx)
		if err != nil {
			return false, err
		}
		if len(models) == 0 {
			fmt.Println("no models found")
			return false, nil
		}
		fmt.Println("available models:")
		for _, m := range models {
			marker := " "
			if m.Name == s.Model {
				marker = "*"
			}
			fmt.Printf("%s %s\n", marker, m.Name)
		}
		return false, nil
	case "/system":
		if arg == "" {
			return false, errors.New("usage: /system <text>")
		}
		s.System = opts.BuildSystemPrompt(arg, opts.ToolsEnabled)
		s.Reset()
		fmt.Println("system prompt updated and history reset")
		return false, nil
	case "/help":
		fmt.Println("/exit | /quit  : end session")
		fmt.Println("/model <name>  : switch model")
		fmt.Println("/models        : list available models")
		fmt.Println("/system <text> : change system prompt and reset history")
		fmt.Println("/reset         : clear chat history")
		fmt.Println("/save          : save current session file")
		fmt.Println("/load          : load current session file")
		fmt.Println("/checkpoint    : create rollback checkpoint")
		fmt.Println("/undo          : restore latest checkpoint")
		fmt.Println("/plan <step>   : add plan item")
		fmt.Println("/plan-list     : show plan")
		fmt.Println("/plan-doing N  : mark item N in progress")
		fmt.Println("/plan-done N   : mark item N completed")
		return false, nil
	case "/save":
		if err := SaveSessionSnapshot(opts.SessionPath, s, opts.ChatHost); err != nil {
			return false, err
		}
		fmt.Printf("session saved: %s\n", opts.SessionPath)
		return false, nil
	case "/load":
		if err := LoadSession(opts.SessionPath, s); err != nil {
			return false, err
		}
		fmt.Printf("session loaded: %s\n", opts.SessionPath)
		return false, nil
	case "/checkpoint":
		id, err := checkpoints.Create()
		if err != nil {
			return false, err
		}
		fmt.Printf("checkpoint created: %s\n", id)
		return false, nil
	case "/undo":
		id, err := checkpoints.RestoreLatest()
		if err != nil {
			return false, err
		}
		fmt.Printf("restored checkpoint: %s\n", id)
		return false, nil
	case "/plan":
		if arg == "" {
			return false, errors.New("usage: /plan <step>")
		}
		planner.Add(arg)
		fmt.Println("plan item added")
		return false, nil
	case "/plan-list":
		fmt.Print(planner.Render())
		return false, nil
	case "/plan-doing":
		idx, err := ParsePositiveInt(arg)
		if err != nil {
			return false, err
		}
		return false, planner.Set(idx, plan.InProgress)
	case "/plan-done":
		idx, err := ParsePositiveInt(arg)
		if err != nil {
			return false, err
		}
		return false, planner.Set(idx, plan.Completed)
	default:
		return false, fmt.Errorf("unknown command: %s", name)
	}
}
