package modelprofile

import "fmt"

type Profile struct {
	Model       string
	System      string
	ToolProfile string
	Retries     int
}

var presets = map[string]Profile{
	"coding-fast": {
		Model:       "qwen2.5-coder:14b",
		System:      "You are a fast coding assistant. Prefer concise answers and minimal tool calls.",
		ToolProfile: "workspace-write",
		Retries:     1,
	},
	"coding-safe": {
		Model:       "qwen2.5-coder:14b",
		System:      "You are a careful coding assistant. Validate changes and explain risks briefly.",
		ToolProfile: "read-only",
		Retries:     2,
	},
	"research": {
		Model:       "qwen2.5:14b-instruct",
		System:      "You are a research assistant. Use web_search when freshness matters and cite sources.",
		ToolProfile: "full",
		Retries:     2,
	},
}

func Apply(name string, chatModel, systemText, toolProfile *string, retries *int, changed func(name string) bool) error {
	if name == "" {
		return nil
	}
	p, ok := presets[name]
	if !ok {
		return fmt.Errorf("unknown model profile: %s", name)
	}
	if !changed("model") {
		*chatModel = p.Model
	}
	if !changed("system") {
		*systemText = p.System
	}
	if !changed("tool-profile") {
		*toolProfile = p.ToolProfile
	}
	if !changed("retries") {
		*retries = p.Retries
	}
	return nil
}

func Names() []string {
	return []string{"coding-fast", "coding-safe", "research"}
}
