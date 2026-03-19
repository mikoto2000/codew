package chatloop

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/peterh/liner"

	"github.com/mikoto2000/codew/internal/session"
)

func LoadLineHistory(lineEditor *liner.State, path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()
	_, err = lineEditor.ReadHistory(file)
	return err
}

func SaveLineHistory(lineEditor *liner.State, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = lineEditor.WriteHistory(file)
	return err
}

func SaveSessionSnapshot(path string, s *session.Session, host string) error {
	return session.SaveToFile(path, s.Snapshot(host))
}

func ParsePositiveInt(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("index is required")
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, errors.New("index must be positive integer")
	}
	return n, nil
}
