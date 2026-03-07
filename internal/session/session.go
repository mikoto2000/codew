package session

import "ollama-codex-cli/internal/ollama"

type Session struct {
	Model   string
	System  string
	history []ollama.Message
}

type Snapshot struct {
	Model   string           `json:"model"`
	System  string           `json:"system"`
	History []ollama.Message `json:"history"`
}

func New(model, system string) *Session {
	s := &Session{Model: model, System: system}
	s.Reset()
	return s
}

func (s *Session) Reset() {
	s.history = []ollama.Message{{Role: "system", Content: s.System}}
}

func (s *Session) AddUser(content string) {
	s.history = append(s.history, ollama.Message{Role: "user", Content: content})
}

func (s *Session) AddAssistantMessage(msg ollama.Message) {
	msg.Role = "assistant"
	s.history = append(s.history, msg)
}

func (s *Session) AddTool(name, toolCallID, content string) {
	s.history = append(s.history, ollama.Message{
		Role:       "tool",
		Name:       name,
		ToolCallID: toolCallID,
		Content:    content,
	})
}

func (s *Session) RollbackLastUser() {
	if len(s.history) == 0 {
		return
	}
	last := s.history[len(s.history)-1]
	if last.Role == "user" {
		s.history = s.history[:len(s.history)-1]
	}
}

func (s *Session) Messages() []ollama.Message {
	out := make([]ollama.Message, len(s.history))
	copy(out, s.history)
	return out
}

func (s *Session) Snapshot() Snapshot {
	return Snapshot{
		Model:   s.Model,
		System:  s.System,
		History: s.Messages(),
	}
}

func (s *Session) Restore(snap Snapshot) {
	s.Model = snap.Model
	s.System = snap.System
	if len(snap.History) == 0 {
		s.Reset()
		return
	}
	s.history = make([]ollama.Message, len(snap.History))
	copy(s.history, snap.History)
}
