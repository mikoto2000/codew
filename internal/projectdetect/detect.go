package projectdetect

import (
	"os"
	"path/filepath"
	"strings"
)

type Result struct {
	Primary string
	All     []string
}

func Detect(workspace string) Result {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return Result{Primary: "unknown", All: []string{"unknown"}}
	}

	markers := map[string]bool{}
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		markers[name] = true
	}

	types := []string{}
	add := func(name string) {
		for _, t := range types {
			if t == name {
				return
			}
		}
		types = append(types, name)
	}

	if markers["go.mod"] {
		add("go")
	}
	if markers["package.json"] {
		add("nodejs")
	}
	if markers["pyproject.toml"] || markers["requirements.txt"] {
		add("python")
	}
	if markers["cargo.toml"] {
		add("rust")
	}
	if markers["pom.xml"] || markers["build.gradle"] || markers["build.gradle.kts"] {
		add("java")
	}
	if markers["gemfile"] {
		add("ruby")
	}
	if markers["composer.json"] {
		add("php")
	}
	if markers["next.config.js"] || markers["next.config.mjs"] || markers["next.config.ts"] {
		add("nextjs")
	}
	if markers["vite.config.js"] || markers["vite.config.ts"] || markers["vite.config.mjs"] {
		add("vite")
	}
	if markers["dockerfile"] || markers["docker-compose.yml"] || markers["docker-compose.yaml"] {
		add("docker")
	}
	if markers["makefile"] {
		add("make")
	}

	if len(types) == 0 {
		// Lightweight fallback: detect by common source extensions in root.
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			switch ext {
			case ".go":
				add("go")
			case ".py":
				add("python")
			case ".rs":
				add("rust")
			case ".ts", ".tsx", ".js", ".jsx":
				add("nodejs")
			}
		}
	}

	if len(types) == 0 {
		return Result{Primary: "unknown", All: []string{"unknown"}}
	}
	return Result{Primary: types[0], All: types}
}
