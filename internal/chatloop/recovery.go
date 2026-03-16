package chatloop

import (
	"fmt"

	"ollama-codex-cli/internal/session"
)

func ResumeSession(path string, s *session.Session) (bool, error) {
	snap, err := session.LoadFromFile(path)
	if err != nil {
		return false, err
	}
	s.Restore(snap)
	return true, nil
}

func LoadSession(path string, s *session.Session) error {
	snap, err := session.LoadFromFile(path)
	if err != nil {
		return err
	}
	s.Restore(snap)
	return nil
}

func ResumeMessage(path string) string {
	return fmt.Sprintf("Resumed session from %s", path)
}
