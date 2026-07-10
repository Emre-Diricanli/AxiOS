package mcp

import (
	"strings"
	"testing"
)

func newTestServer(t *testing.T) (*Server, map[string]*int) {
	t.Helper()
	s := NewServer("axios-test", "0.0.1")
	calls := map[string]*int{
		"read_thing":  new(int),
		"nuke_thing":  new(int),
		"write_thing": new(int),
	}

	register := func(name, permission string) {
		s.RegisterTool(ToolDefinition{
			Name:        name,
			Description: name,
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Permission:  permission,
		}, func(params map[string]any) (string, error) {
			*calls[name]++
			return "ok:" + name, nil
		})
	}
	register("read_thing", "trusted")
	register("nuke_thing", "prohibited")
	register("write_thing", "approval_required")

	return s, calls
}

func TestHandleToolCallProhibited(t *testing.T) {
	tests := []struct {
		name        string
		tool        string
		wantErr     bool // Response.Error set (protocol error)
		wantIsError bool // ToolResult.IsError set
		wantCalls   int
		wantContent string
	}{
		{
			name:        "prohibited tool is rejected without executing",
			tool:        "nuke_thing",
			wantIsError: true,
			wantCalls:   0,
			wantContent: "prohibited by permission policy",
		},
		{
			name:        "trusted tool executes",
			tool:        "read_thing",
			wantCalls:   1,
			wantContent: "ok:read_thing",
		},
		{
			name:        "approval_required tool still executes at this layer",
			tool:        "write_thing",
			wantCalls:   1,
			wantContent: "ok:write_thing",
		},
		{
			name:    "unknown tool is a protocol error",
			tool:    "missing_thing",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, calls := newTestServer(t)

			resp := s.handleRequest(Request{
				Method: "tools/call",
				ID:     "req-1",
				Params: map[string]any{
					"name":   tt.tool,
					"params": map[string]any{},
				},
			})

			if tt.wantErr {
				if resp.Error == nil {
					t.Fatalf("Response.Error = nil, want protocol error")
				}
				return
			}
			if resp.Error != nil {
				t.Fatalf("Response.Error = %+v, want nil", resp.Error)
			}

			result, ok := resp.Result.(ToolResult)
			if !ok {
				t.Fatalf("Response.Result is %T, want ToolResult", resp.Result)
			}
			if result.IsError != tt.wantIsError {
				t.Errorf("ToolResult.IsError = %v, want %v", result.IsError, tt.wantIsError)
			}
			if !strings.Contains(result.Content, tt.wantContent) {
				t.Errorf("ToolResult.Content = %q, want it to contain %q", result.Content, tt.wantContent)
			}
			if counter, tracked := calls[tt.tool]; tracked && *counter != tt.wantCalls {
				t.Errorf("handler invoked %d times, want %d", *counter, tt.wantCalls)
			}
		})
	}
}
