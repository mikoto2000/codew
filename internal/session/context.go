package session

import (
	"fmt"
	"strings"

	"github.com/mikoto2000/codew/internal/ollama"
)

const (
	defaultTailMessages = 6
	summaryMaxChars     = 6000
	entryMaxChars       = 180
)

func (s *Session) MessagesForModel(maxChars int) []ollama.Message {
	msgs := s.Messages()
	if maxChars <= 0 || totalChars(msgs) <= maxChars {
		return msgs
	}
	if len(msgs) <= 2 {
		return msgs
	}

	tailStart := len(msgs) - defaultTailMessages
	if tailStart < 1 {
		tailStart = 1
	}

	head := msgs[0]
	older := msgs[1:tailStart]
	tail := msgs[tailStart:]

	summary := buildSummary(older)
	out := []ollama.Message{head, ollama.Message{Role: "system", Content: summary}}
	out = append(out, tail...)

	for totalChars(out) > maxChars && len(out) > 3 {
		// Drop oldest tail messages first, while preserving system + summary + newest message.
		out = append(out[:2], out[3:]...)
	}

	for totalChars(out) > maxChars && len(out[1].Content) > 200 {
		out[1].Content = out[1].Content[:len(out[1].Content)-200]
	}

	return out
}

func buildSummary(msgs []ollama.Message) string {
	if len(msgs) == 0 {
		return "Earlier conversation summary: (none)"
	}

	var b strings.Builder
	b.WriteString("Earlier conversation summary:\n")
	for _, m := range msgs {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		content = normalizeWhitespace(content)
		if len(content) > entryMaxChars {
			content = content[:entryMaxChars] + "..."
		}
		fmt.Fprintf(&b, "- %s: %s\n", m.Role, content)
		if b.Len() >= summaryMaxChars {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func normalizeWhitespace(s string) string {
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}

func totalChars(msgs []ollama.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Role) + len(m.Name) + len(m.ToolCallID) + len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Name) + len(tc.Function.Arguments)
		}
	}
	return total
}
