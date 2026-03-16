package app

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mikoto2000/codew/internal/checkpoint"
	"github.com/mikoto2000/codew/internal/logging"
	"github.com/mikoto2000/codew/internal/mcp"
	"github.com/mikoto2000/codew/internal/modelprofile"
	"github.com/mikoto2000/codew/internal/ollama"
	"github.com/mikoto2000/codew/internal/projectdetect"
	"github.com/mikoto2000/codew/internal/tools"
)

type ExecuteOptions struct {
	ModelProfile  string
	ToolProfile   string
	Model         string
	System        string
	Retries       int
	WorkspaceRoot string
	SandboxMode   string
	DryRun        bool
	MCPEnabled    bool
	MCPConfig     string
	ChatHost      string
	Timeout       time.Duration
	ToolLogFile   string
	TraceLogFile  string
}

type ExecuteDeps struct {
	Workspace   string
	Project     projectdetect.Result
	Model       string
	System      string
	Retries     int
	Profile     string
	Sandbox     string
	Client      *ollama.Client
	MCPManager  *mcp.Manager
	Executor    *tools.Executor
	Checkpoints *checkpoint.Manager
	ToolLogger  *logging.ToolLogger
	TurnLogger  *logging.TurnLogger
	ToolDefs    []ollama.ToolDefinition
	Allowed     map[string]struct{}
}

func Prepare(cmdFlagChanged func(string) bool, opts ExecuteOptions) (ExecuteDeps, func(), error) {
	if err := modelprofile.Apply(opts.ModelProfile, &opts.Model, &opts.System, &opts.ToolProfile, &opts.Retries, cmdFlagChanged); err != nil {
		return ExecuteDeps{}, nil, err
	}

	workspaceAbs, err := filepath.Abs(opts.WorkspaceRoot)
	if err != nil {
		return ExecuteDeps{}, nil, err
	}
	profile := tools.NormalizeProfile(opts.ToolProfile)
	sandbox := tools.NormalizeSandboxMode(opts.SandboxMode)
	project := projectdetect.Detect(workspaceAbs)
	client := ollama.NewClient(opts.ChatHost, opts.Timeout)
	mcpManager := mcp.NewManager()
	if opts.MCPEnabled {
		mcpCtx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		err = mcpManager.LoadAndStart(mcpCtx, workspaceAbs, opts.MCPConfig)
		cancel()
		if err != nil {
			return ExecuteDeps{}, nil, fmt.Errorf("load mcp tools: %w", err)
		}
	}
	cleanup := func() {
		if opts.MCPEnabled {
			mcpManager.Close()
		}
	}

	executor, err := tools.NewExecutor(workspaceAbs, profile, opts.DryRun, sandbox, mcpManager)
	if err != nil {
		cleanup()
		return ExecuteDeps{}, nil, err
	}
	checkpoints := checkpoint.New(workspaceAbs)

	logPath := opts.ToolLogFile
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(workspaceAbs, logPath)
	}
	tracePath := opts.TraceLogFile
	if !filepath.IsAbs(tracePath) {
		tracePath = filepath.Join(workspaceAbs, tracePath)
	}

	toolDefs := []ollama.ToolDefinition(nil)
	allowed := map[string]struct{}{}
	toolDefs = tools.DefinitionsForProfile(profile)
	toolDefs = append(toolDefs, mcpManager.Definitions()...)
	allowed = tools.AllowedToolNamesForProfile(profile)
	for _, def := range mcpManager.Definitions() {
		allowed[def.Function.Name] = struct{}{}
	}

	return ExecuteDeps{
		Workspace:   workspaceAbs,
		Project:     project,
		Model:       opts.Model,
		System:      opts.System,
		Retries:     opts.Retries,
		Profile:     profile,
		Sandbox:     sandbox,
		Client:      client,
		MCPManager:  mcpManager,
		Executor:    executor,
		Checkpoints: checkpoints,
		ToolLogger:  logging.NewToolLogger(logPath),
		TurnLogger:  logging.NewTurnLogger(tracePath),
		ToolDefs:    toolDefs,
		Allowed:     allowed,
	}, cleanup, nil
}
