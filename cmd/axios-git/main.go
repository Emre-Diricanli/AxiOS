package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/axios-os/axios/internal/gitctl"
	"github.com/axios-os/axios/pkg/logging"
	"github.com/axios-os/axios/pkg/mcp"
)

func main() {
	socketPath := flag.String("socket", mcp.SocketPath("axios-git"), "Unix socket path")
	flag.Parse()

	logger := logging.New("axios-git")

	// Surface a missing git CLI at startup; handlers also guard per call, so
	// the server keeps serving clear errors instead of crashing.
	if err := gitctl.Available(); err != nil {
		logger.Warn("git CLI not found; git tools will fail until it is installed", "error", err)
	}

	server := mcp.NewServer("axios-git", "0.1.0")

	// --- git_status ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_status",
		Description: "Show working tree status of a git repository: current branch, ahead/behind counts, and staged/modified/untracked files",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{
					"type":        "string",
					"description": "Absolute path to the repository (a leading ~ is expanded)",
				},
			},
			"required": []string{"repo"},
		},
		Permission: "trusted",
	}, handleGitStatus)

	// --- git_log ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_log",
		Description: "Show recent commit history of a git repository",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{
					"type":        "string",
					"description": "Absolute path to the repository (a leading ~ is expanded)",
				},
				"count": map[string]any{
					"type":        "integer",
					"description": "Number of commits to return, 1-50 (default 10)",
				},
			},
			"required": []string{"repo"},
		},
		Permission: "trusted",
	}, handleGitLog)

	// --- git_diff ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_diff",
		Description: "Show a diffstat of pending changes, or the full patch for a single file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{
					"type":        "string",
					"description": "Absolute path to the repository (a leading ~ is expanded)",
				},
				"staged": map[string]any{
					"type":        "boolean",
					"description": "Diff the staged changes instead of the working tree (default false)",
				},
				"file": map[string]any{
					"type":        "string",
					"description": "Repository-relative file to show the full patch for (optional)",
				},
			},
			"required": []string{"repo"},
		},
		Permission: "trusted",
	}, handleGitDiff)

	// --- git_branches ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_branches",
		Description: "List local and remote branches with the current branch marked",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{
					"type":        "string",
					"description": "Absolute path to the repository (a leading ~ is expanded)",
				},
			},
			"required": []string{"repo"},
		},
		Permission: "trusted",
	}, handleGitBranches)

	// --- git_show ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_show",
		Description: "Show the author, date, subject, and diffstat of a commit",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{
					"type":        "string",
					"description": "Absolute path to the repository (a leading ~ is expanded)",
				},
				"ref": map[string]any{
					"type":        "string",
					"description": "Commit hash, branch, or tag to show (default HEAD)",
				},
			},
			"required": []string{"repo"},
		},
		Permission: "trusted",
	}, handleGitShow)

	// --- git_remotes ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_remotes",
		Description: "List configured remotes with their fetch and push URLs",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{
					"type":        "string",
					"description": "Absolute path to the repository (a leading ~ is expanded)",
				},
			},
			"required": []string{"repo"},
		},
		Permission: "trusted",
	}, handleGitRemotes)

	// --- git_pull ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_pull",
		Description: "Pull the current branch fast-forward-only (never merges or rebases implicitly)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{
					"type":        "string",
					"description": "Absolute path to the repository (a leading ~ is expanded)",
				},
			},
			"required": []string{"repo"},
		},
		Permission: "approval_required",
	}, handleGitPull)

	// --- git_clone ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_clone",
		Description: "Clone a repository over https://, ssh://, or git@host:path into a new directory",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "Clone URL, e.g. https://github.com/user/repo.git",
				},
				"dir": map[string]any{
					"type":        "string",
					"description": "Absolute path of the directory to clone into (a leading ~ is expanded)",
				},
			},
			"required": []string{"url", "dir"},
		},
		Permission: "approval_required",
	}, handleGitClone)

	// --- git_checkout ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_checkout",
		Description: "Switch to a branch (create=true creates it first)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{
					"type":        "string",
					"description": "Absolute path to the repository (a leading ~ is expanded)",
				},
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch name to switch to",
				},
				"create": map[string]any{
					"type":        "boolean",
					"description": "Create the branch before switching (default false)",
				},
			},
			"required": []string{"repo", "branch"},
		},
		Permission: "approval_required",
	}, handleGitCheckout)

	// --- git_commit ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_commit",
		Description: "Stage all changes and create a commit, returning the new commit hash",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{
					"type":        "string",
					"description": "Absolute path to the repository (a leading ~ is expanded)",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Commit message (must not be empty)",
				},
			},
			"required": []string{"repo", "message"},
		},
		Permission: "approval_required",
	}, handleGitCommit)

	// --- git_push ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "git_push",
		Description: "Push the current branch to its upstream (force pushing is not supported)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"repo": map[string]any{
					"type":        "string",
					"description": "Absolute path to the repository (a leading ~ is expanded)",
				},
				"remote": map[string]any{
					"type":        "string",
					"description": "Remote to push to, e.g. origin (optional)",
				},
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to push (optional, requires remote)",
				},
			},
			"required": []string{"repo"},
		},
		Permission: "approval_required",
	}, handleGitPush)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig.String())
		server.Close()
		os.Exit(0)
	}()

	logger.Info("starting axios-git MCP server", "socket", *socketPath)
	if err := server.Serve(*socketPath); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// --- Tool handlers ---
// Handler errors become IsError tool results in pkg/mcp, so a missing git
// CLI or an invalid path/ref/URL surfaces as a clear error message instead
// of crashing the server.

// requireString extracts a required non-empty string parameter.
func requireString(params map[string]any, key string) (string, error) {
	v, ok := params[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	return v, nil
}

func handleGitStatus(params map[string]any) (string, error) {
	repo, err := requireString(params, "repo")
	if err != nil {
		return "", err
	}

	status, err := gitctl.Status(repo)
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(status)
	if err != nil {
		return "", fmt.Errorf("marshal status: %w", err)
	}
	return fmt.Sprintf("status of %s:\n%s", repo, out), nil
}

func handleGitLog(params map[string]any) (string, error) {
	repo, err := requireString(params, "repo")
	if err != nil {
		return "", err
	}

	count := 10
	if c, ok := params["count"].(float64); ok && int(c) > 0 {
		count = int(c)
	}

	commits, err := gitctl.Log(repo, count)
	if err != nil {
		return "", err
	}
	if len(commits) == 0 {
		return "no commits found", nil
	}

	out, err := json.Marshal(commits)
	if err != nil {
		return "", fmt.Errorf("marshal commits: %w", err)
	}
	return fmt.Sprintf("%d commits:\n%s", len(commits), out), nil
}

func handleGitDiff(params map[string]any) (string, error) {
	repo, err := requireString(params, "repo")
	if err != nil {
		return "", err
	}
	staged, _ := params["staged"].(bool)
	file, _ := params["file"].(string)

	output, err := gitctl.Diff(repo, staged, file)
	if err != nil {
		return "", err
	}
	if output == "" {
		if staged {
			return "no staged changes", nil
		}
		return "no unstaged changes", nil
	}
	return output, nil
}

func handleGitBranches(params map[string]any) (string, error) {
	repo, err := requireString(params, "repo")
	if err != nil {
		return "", err
	}

	branches, err := gitctl.Branches(repo)
	if err != nil {
		return "", err
	}
	if len(branches) == 0 {
		return "no branches found", nil
	}

	out, err := json.Marshal(branches)
	if err != nil {
		return "", fmt.Errorf("marshal branches: %w", err)
	}
	return fmt.Sprintf("%d branches:\n%s", len(branches), out), nil
}

func handleGitShow(params map[string]any) (string, error) {
	repo, err := requireString(params, "repo")
	if err != nil {
		return "", err
	}

	ref := "HEAD"
	if r, ok := params["ref"].(string); ok && r != "" {
		ref = r
	}

	output, err := gitctl.Show(repo, ref)
	if err != nil {
		return "", err
	}
	if output == "" {
		return fmt.Sprintf("no output for ref %s", ref), nil
	}
	return output, nil
}

func handleGitRemotes(params map[string]any) (string, error) {
	repo, err := requireString(params, "repo")
	if err != nil {
		return "", err
	}

	remotes, err := gitctl.Remotes(repo)
	if err != nil {
		return "", err
	}
	if len(remotes) == 0 {
		return "no remotes configured", nil
	}

	out, err := json.Marshal(remotes)
	if err != nil {
		return "", fmt.Errorf("marshal remotes: %w", err)
	}
	return fmt.Sprintf("%d remotes:\n%s", len(remotes), out), nil
}

func handleGitPull(params map[string]any) (string, error) {
	repo, err := requireString(params, "repo")
	if err != nil {
		return "", err
	}

	output, err := gitctl.Pull(repo)
	if err != nil {
		return "", err
	}
	if output == "" {
		return "pull complete", nil
	}
	return output, nil
}

func handleGitClone(params map[string]any) (string, error) {
	url, err := requireString(params, "url")
	if err != nil {
		return "", err
	}
	dir, err := requireString(params, "dir")
	if err != nil {
		return "", err
	}

	output, err := gitctl.Clone(url, dir)
	if err != nil {
		return "", err
	}
	if output == "" {
		return fmt.Sprintf("cloned %s into %s", url, dir), nil
	}
	return output, nil
}

func handleGitCheckout(params map[string]any) (string, error) {
	repo, err := requireString(params, "repo")
	if err != nil {
		return "", err
	}
	branch, err := requireString(params, "branch")
	if err != nil {
		return "", err
	}
	create, _ := params["create"].(bool)

	output, err := gitctl.Checkout(repo, branch, create)
	if err != nil {
		return "", err
	}
	if output == "" {
		return fmt.Sprintf("switched to branch %s", branch), nil
	}
	return output, nil
}

func handleGitCommit(params map[string]any) (string, error) {
	repo, err := requireString(params, "repo")
	if err != nil {
		return "", err
	}
	message, err := requireString(params, "message")
	if err != nil {
		return "", err
	}

	hash, err := gitctl.CommitAll(repo, message)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("committed %s", hash), nil
}

func handleGitPush(params map[string]any) (string, error) {
	repo, err := requireString(params, "repo")
	if err != nil {
		return "", err
	}
	remote, _ := params["remote"].(string)
	branch, _ := params["branch"].(string)

	output, err := gitctl.Push(repo, remote, branch)
	if err != nil {
		return "", err
	}
	if output == "" {
		return "push complete", nil
	}
	return output, nil
}
