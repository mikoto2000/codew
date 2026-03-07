package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type ToolEvent struct {
	Timestamp string `json:"timestamp"`
	TurnInput string `json:"turn_input,omitempty"`
	Tool      string `json:"tool"`
	Args      string `json:"args,omitempty"`
	Result    string `json:"result,omitempty"`
	Approved  bool   `json:"approved"`
}

type ToolLogger struct {
	path string
}

func NewToolLogger(path string) *ToolLogger {
	return &ToolLogger{path: path}
}

func (l *ToolLogger) Append(event ToolEvent) error {
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
