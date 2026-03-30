package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/axios-os/axios/pkg/mcp"
)

func main() {
	defaultSocket := mcp.SocketPath("axios-fs")
	socket := flag.String("socket", defaultSocket, "Unix socket path to listen on")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	server := mcp.NewServer("axios-fs", "0.1.0")

	// ── read_file ───────────────────────────────────────────────────────
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "read_file",
		Description: "Read file contents at a given path.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file to read.",
				},
			},
			"required": []string{"path"},
		},
		Permission: "trusted",
	}, handleReadFile)

	// ── write_file ──────────────────────────────────────────────────────
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "write_file",
		Description: "Write content to a file. Creates parent directories if needed.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file to write.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write to the file.",
				},
			},
			"required": []string{"path", "content"},
		},
		Permission: "trusted",
	}, handleWriteFile)

	// ── list_directory ──────────────────────────────────────────────────
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "list_directory",
		Description: "List files and directories at a path.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the directory to list.",
				},
			},
			"required": []string{"path"},
		},
		Permission: "trusted",
	}, handleListDirectory)

	// ── search_files ────────────────────────────────────────────────────
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "search_files",
		Description: "Search for files by name pattern using filepath.Glob syntax.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"directory": map[string]any{
					"type":        "string",
					"description": "Directory to search in.",
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "Glob pattern to match file names (e.g. \"*.go\", \"config.*\").",
				},
			},
			"required": []string{"directory", "pattern"},
		},
		Permission: "trusted",
	}, handleSearchFiles)

	// ── file_info ───────────────────────────────────────────────────────
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "file_info",
		Description: "Get file metadata including size, permissions, and modification time.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file or directory.",
				},
			},
			"required": []string{"path"},
		},
		Permission: "trusted",
	}, handleFileInfo)

	// ── delete_file ─────────────────────────────────────────────────────
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "delete_file",
		Description: "Delete a file or empty directory.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file or empty directory to delete.",
				},
			},
			"required": []string{"path"},
		},
		Permission: "approval_required",
	}, handleDeleteFile)

	// ── Start server ────────────────────────────────────────────────────
	logger.Info("axios-fs starting", "socket", *socket)
	if err := server.Serve(*socket); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// ── Tool handlers ───────────────────────────────────────────────────────────

func handleReadFile(params map[string]any) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing required parameter: path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	return string(data), nil
}

func handleWriteFile(params map[string]any) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing required parameter: path")
	}

	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("missing required parameter: content")
	}

	// Create parent directories if they don't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create parent directories: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return fmt.Sprintf("wrote %d bytes to %s", len(content), path), nil
}

func handleListDirectory(params map[string]any) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing required parameter: path")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("list directory: %w", err)
	}

	type dirEntry struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Size int64  `json:"size"`
	}

	result := make([]dirEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entryType := "file"
		if entry.IsDir() {
			entryType = "dir"
		}
		result = append(result, dirEntry{
			Name: entry.Name(),
			Type: entryType,
			Size: info.Size(),
		})
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}

	return string(out), nil
}

func handleSearchFiles(params map[string]any) (string, error) {
	directory, ok := params["directory"].(string)
	if !ok || directory == "" {
		return "", fmt.Errorf("missing required parameter: directory")
	}

	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("missing required parameter: pattern")
	}

	globPattern := filepath.Join(directory, pattern)
	matches, err := filepath.Glob(globPattern)
	if err != nil {
		return "", fmt.Errorf("glob search: %w", err)
	}

	if matches == nil {
		matches = []string{}
	}

	out, err := json.MarshalIndent(matches, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}

	return string(out), nil
}

func handleFileInfo(params map[string]any) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing required parameter: path")
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}

	fileType := "file"
	if info.IsDir() {
		fileType = "dir"
	}

	result := map[string]any{
		"name":        info.Name(),
		"size":        info.Size(),
		"type":        fileType,
		"permissions": info.Mode().String(),
		"mod_time":    info.ModTime().Format(time.RFC3339),
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}

	return string(out), nil
}

func handleDeleteFile(params map[string]any) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing required parameter: path")
	}

	// Check that the target exists before attempting removal
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}

	// Use os.Remove which deletes files and empty directories only
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("delete: %w", err)
	}

	if info.IsDir() {
		return fmt.Sprintf("deleted empty directory: %s", path), nil
	}
	return fmt.Sprintf("deleted file: %s", path), nil
}
