package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function ToolFunctionCall `json:"function"`
}

type ToolFunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ToolDefinition struct {
	Type     string             `json:"type"`
	Function ToolDefinitionFunc `json:"function"`
}

type ToolDefinitionFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type chatRequest struct {
	Model    string           `json:"model"`
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
}

type chatResponse struct {
	Message Message `json:"message"`
	Error   string  `json:"error,omitempty"`
}

type Client struct {
	host string
	http *http.Client
}

func NewClient(host string, timeout time.Duration) *Client {
	return &Client{
		host: strings.TrimRight(host, "/"),
		http: &http.Client{Timeout: timeout},
	}
}

func (c *Client) Chat(ctx context.Context, model string, messages []Message, tools []ToolDefinition) (Message, error) {
	payload := chatRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return Message{}, fmt.Errorf("marshal request: %w", err)
	}

	url := c.host + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return Message{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return Message{}, fmt.Errorf("ollama status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Message{}, fmt.Errorf("decode response: %w", err)
	}
	if out.Error != "" {
		return Message{}, fmt.Errorf("ollama error: %s", out.Error)
	}
	return out.Message, nil
}
