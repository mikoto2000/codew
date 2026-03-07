package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type TurnEvent struct {
	Timestamp  string `json:"timestamp"`
	Mode       string `json:"mode"`
	Input      string `json:"input,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	ToolCalls  int    `json:"tool_calls"`
	Error      string `json:"error,omitempty"`
}

type TurnLogger struct {
	path string
}

func NewTurnLogger(path string) *TurnLogger {
	return &TurnLogger{path: path}
}

func (l *TurnLogger) Append(event TurnEvent) error {
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
