package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/axios-os/axios/internal/obsidianctl"
	"github.com/axios-os/axios/pkg/logging"
	"github.com/axios-os/axios/pkg/mcp"
)

// manager resolves the active vault on every call (state file written by the
// daemon → optional --vault override), so a vault switch made in the web UI
// reaches this process without a restart.
var manager *obsidianctl.Manager

func main() {
	socketPath := flag.String("socket", mcp.SocketPath("axios-obsidian"), "Unix socket path")
	vaultPath := flag.String("vault", "", "vault directory override (default: the vault configured in AxiOS)")
	flag.Parse()

	logger := logging.New("axios-obsidian")

	// Same data-dir resolution as axiosd: the obsidian.json state file the
	// daemon writes on PUT /api/obsidian/vault lives here.
	dataDir := os.Getenv("AXIOS_DATA_DIR")
	if dataDir == "" {
		homeDir, _ := os.UserHomeDir()
		dataDir = filepath.Join(homeDir, ".axios")
	}

	manager = obsidianctl.NewManager(dataDir, "")
	if *vaultPath != "" {
		manager.SetOverride(*vaultPath)
	}

	// Surface an unusable vault at startup; handlers also guard per call, so
	// the server keeps serving clear errors instead of crashing.
	if _, err := manager.Vault(); err != nil {
		logger.Warn("Obsidian vault not usable yet; tools will explain how to configure one", "error", err)
	}

	server := mcp.NewServer("axios-obsidian", "0.1.0")

	// --- search_notes ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "search_notes",
		Description: "Search the user's Obsidian vault (personal Markdown notes) by case-insensitive substring over note names and content, optionally restricted to notes carrying a tag",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Text to search for in note names and content (case-insensitive)",
				},
				"tag": map[string]any{
					"type":        "string",
					"description": "Only return notes carrying this tag, e.g. 'work' (leading '#' optional)",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of hits, 1-100 (default 20)",
				},
			},
			"required": []string{"query"},
		},
		Permission: "trusted",
	}, handleSearchNotes)

	// --- read_note ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "read_note",
		Description: "Read a note from the user's Obsidian vault, returning its content plus parsed frontmatter and tags",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Vault-relative note path, e.g. 'Work/AxiOS/notes.md' ('.md' is added automatically when omitted)",
				},
			},
			"required": []string{"path"},
		},
		Permission: "trusted",
	}, handleReadNote)

	// --- list_notes ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "list_notes",
		Description: "List the notes and folders in the user's Obsidian vault",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"folder": map[string]any{
					"type":        "string",
					"description": "Vault-relative folder to list, e.g. 'Work/AxiOS' (omit for the vault root)",
				},
				"recursive": map[string]any{
					"type":        "boolean",
					"description": "Descend into subfolders (default false)",
				},
			},
		},
		Permission: "trusted",
	}, handleListNotes)

	// --- vault_info ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "vault_info",
		Description: "Show the configured Obsidian vault: root path, name, note and folder counts, and total size",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "trusted",
	}, handleVaultInfo)

	// --- write_note ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "write_note",
		Description: "Create a note in the user's Obsidian vault (parent folders are created automatically; set overwrite to replace an existing note)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Vault-relative note path, e.g. 'Work/AxiOS/notes.md' ('.md' is added automatically when omitted)",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Full Markdown content of the note",
				},
				"overwrite": map[string]any{
					"type":        "boolean",
					"description": "Replace the note if it already exists (default false)",
				},
			},
			"required": []string{"path", "content"},
		},
		Permission: "approval_required",
	}, handleWriteNote)

	// --- append_note ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "append_note",
		Description: "Append a Markdown block to a note in the user's Obsidian vault (the note is created when missing; one blank line separates the existing content from the appended block)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Vault-relative note path, e.g. 'Work/AxiOS/notes.md' ('.md' is added automatically when omitted)",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Markdown block to append",
				},
			},
			"required": []string{"path", "content"},
		},
		Permission: "approval_required",
	}, handleAppendNote)

	// --- delete_note ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "delete_note",
		Description: "Delete a note from the user's Obsidian vault (single notes only — folders are never deleted)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Vault-relative note path, e.g. 'Work/AxiOS/notes.md' ('.md' is added automatically when omitted)",
				},
			},
			"required": []string{"path"},
		},
		Permission: "approval_required",
	}, handleDeleteNote)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig.String())
		server.Close()
		os.Exit(0)
	}()

	logger.Info("starting axios-obsidian MCP server", "socket", *socketPath)
	if err := server.Serve(*socketPath); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// --- Tool handlers ---
// Handler errors become IsError tool results in pkg/mcp, so an unconfigured
// vault or an invalid note path surfaces as a clear error message instead of
// crashing the server.

// requireString extracts a required non-empty string parameter.
func requireString(params map[string]any, key string) (string, error) {
	v, ok := params[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	return v, nil
}

// vault resolves the active vault, translating the unconfigured state into a
// friendly, actionable message for the model to relay to the user.
func vault() (*obsidianctl.Vault, error) {
	v, err := manager.Vault()
	if errors.Is(err, obsidianctl.ErrNotConfigured) {
		return nil, fmt.Errorf("no Obsidian vault is configured — ask the user to set the vault path in Settings → Obsidian in the AxiOS web UI (or obsidian.vault in configs/axiosd.yaml)")
	}
	if err != nil {
		return nil, fmt.Errorf("Obsidian vault unavailable: %w", err)
	}
	return v, nil
}

func handleSearchNotes(params map[string]any) (string, error) {
	v, err := vault()
	if err != nil {
		return "", err
	}
	query, err := requireString(params, "query")
	if err != nil {
		return "", err
	}
	tag, _ := params["tag"].(string)
	limit := 0
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	hits, err := v.Search(query, tag, limit)
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "no notes matched", nil
	}

	out, err := json.Marshal(hits)
	if err != nil {
		return "", fmt.Errorf("marshal hits: %w", err)
	}
	return fmt.Sprintf("%d notes matched:\n%s", len(hits), out), nil
}

func handleReadNote(params map[string]any) (string, error) {
	v, err := vault()
	if err != nil {
		return "", err
	}
	path, err := requireString(params, "path")
	if err != nil {
		return "", err
	}

	note, err := v.ReadNote(path)
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(note)
	if err != nil {
		return "", fmt.Errorf("marshal note: %w", err)
	}
	return string(out), nil
}

func handleListNotes(params map[string]any) (string, error) {
	v, err := vault()
	if err != nil {
		return "", err
	}
	folder, _ := params["folder"].(string)
	recursive, _ := params["recursive"].(bool)

	entries, err := v.ListNotes(folder, recursive)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "no notes or folders found", nil
	}

	out, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("marshal entries: %w", err)
	}
	return fmt.Sprintf("%d entries:\n%s", len(entries), out), nil
}

func handleVaultInfo(params map[string]any) (string, error) {
	v, err := vault()
	if err != nil {
		return "", err
	}

	info, err := v.Info()
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(info)
	if err != nil {
		return "", fmt.Errorf("marshal vault info: %w", err)
	}
	return string(out), nil
}

func handleWriteNote(params map[string]any) (string, error) {
	v, err := vault()
	if err != nil {
		return "", err
	}
	path, err := requireString(params, "path")
	if err != nil {
		return "", err
	}
	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("missing required parameter: content")
	}
	overwrite, _ := params["overwrite"].(bool)

	if err := v.WriteNote(path, content, overwrite); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote note %s", path), nil
}

func handleAppendNote(params map[string]any) (string, error) {
	v, err := vault()
	if err != nil {
		return "", err
	}
	path, err := requireString(params, "path")
	if err != nil {
		return "", err
	}
	content, err := requireString(params, "content")
	if err != nil {
		return "", err
	}

	if err := v.AppendNote(path, content); err != nil {
		return "", err
	}
	return fmt.Sprintf("appended to note %s", path), nil
}

func handleDeleteNote(params map[string]any) (string, error) {
	v, err := vault()
	if err != nil {
		return "", err
	}
	path, err := requireString(params, "path")
	if err != nil {
		return "", err
	}

	if err := v.DeleteNote(path); err != nil {
		return "", err
	}
	return fmt.Sprintf("deleted note %s", path), nil
}
