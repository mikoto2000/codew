package session

import "ollama-codex-cli/internal/ollama"

type Session struct {
	Model   string
	System  string
	history []ollama.Message
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

func (s *Session) AddAssistant(content string) {
	s.history = append(s.history, ollama.Message{Role: "assistant", Content: content})
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
