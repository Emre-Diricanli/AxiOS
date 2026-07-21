package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// Client connects to an MCP server over a Unix socket.
type Client struct {
	conn    net.Conn
	encoder *json.Encoder
	decoder *json.Decoder
	mu      sync.Mutex
	nextID  atomic.Int64
}

// Dial connects to an MCP server at the given Unix socket path.
func Dial(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", socketPath, err)
	}

	return &Client{
		conn:    conn,
		encoder: json.NewEncoder(conn),
		decoder: json.NewDecoder(conn),
	}, nil
}

// Initialize sends the initialize request and returns the server info.
func (c *Client) Initialize() (*ServerInfo, error) {
	resp, err := c.call("initialize", nil)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}

	var info ServerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ListTools returns the tools available on the server.
func (c *Client) ListTools() ([]ToolDefinition, error) {
	resp, err := c.call("tools/list", nil)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}

	var tools []ToolDefinition
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, err
	}
	return tools, nil
}

// CallTool invokes a tool on the server and returns the result.
func (c *Client) CallTool(name string, params map[string]any) (*ToolResult, error) {
	resp, err := c.call("tools/call", map[string]any{
		"name":   name,
		"params": params,
	})
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}

	var result ToolResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) call(method string, params map[string]any) (*Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := fmt.Sprintf("%d", c.nextID.Add(1))
	req := Request{
		Method: method,
		ID:     id,
		Params: params,
	}

	if err := c.encoder.Encode(req); err != nil {
		return nil, &ConnError{Op: "send request", Err: err}
	}

	var resp Response
	if err := c.decoder.Decode(&resp); err != nil {
		return nil, &ConnError{Op: "read response", Err: err}
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp, nil
}

// ConnError marks a connection-level failure (dial, send, receive), as
// opposed to a protocol-level error returned by the server. It lets callers
// tell "the server is gone, reconnect" apart from "the server answered with
// an error".
type ConnError struct {
	Op  string
	Err error
}

func (e *ConnError) Error() string { return fmt.Sprintf("%s: %v", e.Op, e.Err) }
func (e *ConnError) Unwrap() error { return e.Err }

// IsConnError reports whether err is a connection-level failure.
func IsConnError(err error) bool {
	var ce *ConnError
	return errors.As(err, &ce)
}

// Close closes the connection to the MCP server.
func (c *Client) Close() error {
	return c.conn.Close()
}
