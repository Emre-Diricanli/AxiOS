package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		component string
	}{
		{"axiosd daemon", "axiosd"},
		{"mcp server name", "axios-fs"},
		{"empty component", ""},
		{"hyphenated", "axios-telemetry"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// New wires a JSON handler to os.Stdout at construction time.
			// Redirect stdout so we can assert the component attribute.
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("os.Pipe: %v", err)
			}
			orig := os.Stdout
			os.Stdout = w
			defer func() { os.Stdout = orig }()

			logger := New(tt.component)
			if logger == nil {
				t.Fatal("New returned nil logger")
			}

			logger.Info("test message")

			_ = w.Close()
			os.Stdout = orig

			var buf bytes.Buffer
			if _, err := io.Copy(&buf, r); err != nil {
				t.Fatalf("read stdout: %v", err)
			}
			_ = r.Close()

			line := bytes.TrimSpace(buf.Bytes())
			if len(line) == 0 {
				t.Fatal("expected a log line on stdout, got empty output")
			}

			var record map[string]any
			if err := json.Unmarshal(line, &record); err != nil {
				t.Fatalf("log line is not JSON: %v\nline: %s", err, line)
			}

			got, ok := record["component"]
			if !ok {
				t.Fatalf("log record missing component attribute: %s", line)
			}
			gotStr, ok := got.(string)
			if !ok {
				t.Fatalf("component is not a string: %T (%v)", got, got)
			}
			if gotStr != tt.component {
				t.Errorf("component = %q, want %q", gotStr, tt.component)
			}
			if msg, _ := record["msg"].(string); msg != "test message" {
				t.Errorf("msg = %q, want %q", msg, "test message")
			}
		})
	}
}
