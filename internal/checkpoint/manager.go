package checkpoint

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxFileBytes = 2 * 1024 * 1024
)

type fileEntry struct {
	Path    string `json:"path"`
	Content string `json:"content_base64"`
}

type snapshot struct {
	ID        string      `json:"id"`
	CreatedAt time.Time   `json:"created_at"`
	Files     []fileEntry `json:"files"`
}

type indexFile struct {
	IDs []string `json:"ids"`
}

type Manager struct {
	workspace string
	dir       string
}

func New(workspace string) *Manager {
	return &Manager{
		workspace: workspace,
		dir:       filepath.Join(workspace, ".codew", "checkpoints"),
	}
}

func (m *Manager) Create() (string, error) {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return "", err
	}
	files, err := m.captureFiles()
	if err != nil {
		return "", err
	}

	id := time.Now().UTC().Format("20060102-150405")
	snap := snapshot{ID: id, CreatedAt: time.Now().UTC(), Files: files}
	data, err := json.Marshal(snap)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(m.dir, id+".json"), data, 0o644); err != nil {
		return "", err
	}
	idx, err := m.readIndex()
	if err != nil {
		return "", err
	}
	idx.IDs = append(idx.IDs, id)
	if err := m.writeIndex(idx); err != nil {
		return "", err
	}
	return id, nil
}

func (m *Manager) RestoreLatest() (string, error) {
	idx, err := m.readIndex()
	if err != nil {
		return "", err
	}
	if len(idx.IDs) == 0 {
		return "", errors.New("no checkpoint available")
	}
	id := idx.IDs[len(idx.IDs)-1]
	snapPath := filepath.Join(m.dir, id+".json")
	data, err := os.ReadFile(snapPath)
	if err != nil {
		return "", err
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return "", err
	}
	if err := m.restoreSnapshot(snap); err != nil {
		return "", err
	}

	idx.IDs = idx.IDs[:len(idx.IDs)-1]
	if err := m.writeIndex(idx); err != nil {
		return "", err
	}
	return id, nil
}

func (m *Manager) captureFiles() ([]fileEntry, error) {
	out := make([]fileEntry, 0, 128)
	err := filepath.WalkDir(m.workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(m.workspace, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if rel == ".git" || strings.HasPrefix(rel, ".git/") || rel == ".gocache" || strings.HasPrefix(rel, ".gocache/") || rel == ".codew" || strings.HasPrefix(rel, ".codew/") {
				return fs.SkipDir
			}
			return nil
		}

		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		if info.Size() > maxFileBytes {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		out = append(out, fileEntry{Path: rel, Content: base64.StdEncoding.EncodeToString(data)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (m *Manager) restoreSnapshot(s snapshot) error {
	target := map[string][]byte{}
	for _, f := range s.Files {
		decoded, err := base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			return err
		}
		target[f.Path] = decoded
	}

	current := map[string]struct{}{}
	err := filepath.WalkDir(m.workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(m.workspace, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if rel == ".git" || strings.HasPrefix(rel, ".git/") || rel == ".gocache" || strings.HasPrefix(rel, ".gocache/") || rel == ".codew" || strings.HasPrefix(rel, ".codew/") {
				return fs.SkipDir
			}
			return nil
		}
		current[rel] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}

	for rel := range current {
		if _, ok := target[rel]; !ok {
			_ = os.Remove(filepath.Join(m.workspace, rel))
		}
	}
	for rel, data := range target {
		abs := filepath.Join(m.workspace, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) readIndex() (indexFile, error) {
	path := filepath.Join(m.dir, "index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return indexFile{}, nil
		}
		return indexFile{}, err
	}
	var idx indexFile
	if err := json.Unmarshal(data, &idx); err != nil {
		return indexFile{}, fmt.Errorf("decode checkpoint index: %w", err)
	}
	return idx, nil
}

func (m *Manager) writeIndex(idx indexFile) error {
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.dir, "index.json"), data, 0o644)
}
