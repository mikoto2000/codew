package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type TraceEvent struct {
	Timestamp  string `json:"timestamp"`
	Event      string `json:"event"`
	TurnID     string `json:"turn_id,omitempty"`
	Step       int    `json:"step,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Input      string `json:"input,omitempty"`
	Model      string `json:"model,omitempty"`
	Tool       string `json:"tool,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	ToolCalls  int    `json:"tool_calls,omitempty"`
	Error      string `json:"error,omitempty"`
}

type TurnLogger struct {
	path string
}

func NewTurnLogger(path string) *TurnLogger {
	return &TurnLogger{path: path}
}

func (l *TurnLogger) Append(event TraceEvent) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}
