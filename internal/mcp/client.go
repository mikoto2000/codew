package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type Client struct {
	name    string
	cfg     ServerConfig
	process *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	reader  *bufio.Reader
	mu      sync.Mutex
	nextID  int
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type toolsListResult struct {
	Tools []Tool `json:"tools"`
}

type toolsCallResult struct {
	Content []map[string]any `json:"content"`
	IsError bool             `json:"isError"`
}

func NewClient(name string, cfg ServerConfig) *Client {
	return &Client{name: name, cfg: cfg, nextID: 1}
}

func (c *Client) Name() string { return c.name }

func (c *Client) Start(ctx context.Context, workspace string) error {
	cmd := exec.CommandContext(ctx, c.cfg.Command, c.cfg.Args...)
	if c.cfg.Cwd != "" {
		if filepath.IsAbs(c.cfg.Cwd) {
			cmd.Dir = c.cfg.Cwd
		} else {
			cmd.Dir = filepath.Join(workspace, c.cfg.Cwd)
		}
	} else {
		cmd.Dir = workspace
	}
	cmd.Env = os.Environ()
	for k, v := range c.cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	c.process = cmd
	c.stdin = stdin
	c.stdout = stdout
	c.reader = bufio.NewReader(stdout)

	if err := c.initialize(ctx); err != nil {
		_ = c.Close()
		return fmt.Errorf("initialize mcp server %q: %w", c.name, err)
	}
	return nil
}

func (c *Client) Close() error {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.stdout != nil {
		_ = c.stdout.Close()
	}
	if c.process != nil && c.process.Process != nil {
		_ = c.process.Process.Kill()
		_, _ = c.process.Process.Wait()
	}
	return nil
}

func (c *Client) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo": map[string]any{
			"name":    "codew",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{},
	}
	if _, err := c.request(ctx, "initialize", params); err != nil {
		return err
	}
	if err := c.notify("notifications/initialized", map[string]any{}); err != nil {
		return err
	}
	return nil
}

func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	res, err := c.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var parsed toolsListResult
	if err := json.Unmarshal(res, &parsed); err != nil {
		return nil, err
	}
	return parsed.Tools, nil
}

func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	params := map[string]any{"name": name, "arguments": args}
	res, err := c.request(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	var parsed toolsCallResult
	if err := json.Unmarshal(res, &parsed); err != nil {
		return nil, err
	}
	if parsed.IsError {
		return map[string]any{"is_error": true, "content": parsed.Content}, nil
	}
	return map[string]any{"is_error": false, "content": parsed.Content}, nil
}

func (c *Client) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := rpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	return c.writeMessage(msg)
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++

	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	if err := c.writeMessage(req); err != nil {
		return nil, err
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		payload, err := c.readMessage()
		if err != nil {
			return nil, err
		}
		var resp rpcResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			continue
		}
		if len(resp.ID) == 0 {
			continue
		}
		var gotID int
		if err := json.Unmarshal(resp.ID, &gotID); err != nil {
			continue
		}
		if gotID != id {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *Client) writeMessage(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return err
	}
	if _, err := c.stdin.Write(data); err != nil {
		return err
	}
	return nil
}

func (c *Client) readMessage() ([]byte, error) {
	contentLength := -1
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])
		if name == "content-length" {
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, errors.New("invalid content-length")
	}
	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(payload), nil
}
