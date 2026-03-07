package contextloader

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxFileSize = 512 * 1024

type candidate struct {
	path  string
	score int
}

func Build(workspace, query string, maxFiles, maxChars int) (string, error) {
	if maxFiles <= 0 || maxChars <= 0 {
		return "", nil
	}
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return "", nil
	}

	cands := make([]candidate, 0, 64)
	err := filepath.WalkDir(workspace, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(workspace, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			if rel == ".git" || strings.HasPrefix(rel, ".git/") || rel == ".gocache" || strings.HasPrefix(rel, ".gocache/") {
				return fs.SkipDir
			}
			return nil
		}
		if !isTextLike(rel) {
			return nil
		}

		lower := strings.ToLower(rel)
		score := 0
		for _, t := range tokens {
			if strings.Contains(lower, t) {
				score += 3
			}
		}
		if score > 0 {
			cands = append(cands, candidate{path: rel, score: score})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(cands) == 0 {
		return "", nil
	}

	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score == cands[j].score {
			return cands[i].path < cands[j].path
		}
		return cands[i].score > cands[j].score
	})
	if len(cands) > maxFiles {
		cands = cands[:maxFiles]
	}

	var b strings.Builder
	b.WriteString("Auto-loaded project context (most relevant files):\n")
	for _, c := range cands {
		abs := filepath.Join(workspace, c.path)
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			continue
		}
		if len(data) > maxFileSize {
			data = data[:maxFileSize]
		}
		content := string(data)
		if len(content) > maxChars/len(cands) {
			content = content[:maxChars/len(cands)] + "\n...<truncated>"
		}
		fmt.Fprintf(&b, "\n[file] %s\n%s\n", c.path, content)
		if b.Len() > maxChars {
			break
		}
	}

	out := b.String()
	if len(out) > maxChars {
		out = out[:maxChars] + "\n...<truncated>"
	}
	return strings.TrimSpace(out), nil
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	repl := strings.NewReplacer(",", " ", ".", " ", "/", " ", "-", " ", "_", " ", "(", " ", ")", " ", "\t", " ", "\n", " ")
	s = repl.Replace(s)
	parts := strings.Fields(s)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) < 3 {
			continue
		}
		if isStopWord(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func isStopWord(s string) bool {
	switch s {
	case "the", "and", "for", "with", "this", "that", "from", "your", "please", "what", "when", "where", "how", "can", "you", "して", "です", "ます", "ため", "について":
		return true
	default:
		return false
	}
}

func isTextLike(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".sh", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".c", ".cpp", ".h":
		return true
	default:
		base := filepath.Base(path)
		return base == "makefile" || base == "dockerfile"
	}
}
