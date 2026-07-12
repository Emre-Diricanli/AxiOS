package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/axios-os/axios/internal/dockerctl"
	"github.com/axios-os/axios/pkg/logging"
	"github.com/axios-os/axios/pkg/mcp"
)

func main() {
	socketPath := flag.String("socket", mcp.SocketPath("axios-docker"), "Unix socket path")
	flag.Parse()

	logger := logging.New("axios-docker")

	server := mcp.NewServer("axios-docker", "0.1.0")

	// --- list_containers ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "list_containers",
		Description: "List Docker containers (running only by default; set all=true to include stopped)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"all": map[string]any{
					"type":        "boolean",
					"description": "Include stopped containers (default false)",
				},
			},
		},
		Permission: "trusted",
	}, handleListContainers)

	// --- container_info ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "container_info",
		Description: "Inspect a container and return its full configuration as JSON",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Container ID or name",
				},
			},
			"required": []string{"id"},
		},
		Permission: "trusted",
	}, handleContainerInfo)

	// --- container_logs ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "container_logs",
		Description: "Fetch recent logs from a container",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Container ID or name",
				},
				"tail": map[string]any{
					"type":        "integer",
					"description": "Number of log lines to return (default 100)",
				},
			},
			"required": []string{"id"},
		},
		Permission: "trusted",
	}, handleContainerLogs)

	// --- list_images ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "list_images",
		Description: "List local Docker images",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "trusted",
	}, handleListImages)

	// --- docker_stats ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "docker_stats",
		Description: "Get CPU, memory, network, and disk I/O usage per running container",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "trusted",
	}, handleDockerStats)

	// --- start_container ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "start_container",
		Description: "Start a stopped container",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Container ID or name",
				},
			},
			"required": []string{"id"},
		},
		Permission: "approval_required",
	}, handleStartContainer)

	// --- stop_container ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "stop_container",
		Description: "Stop a running container",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Container ID or name",
				},
			},
			"required": []string{"id"},
		},
		Permission: "approval_required",
	}, handleStopContainer)

	// --- restart_container ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "restart_container",
		Description: "Restart a container",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Container ID or name",
				},
			},
			"required": []string{"id"},
		},
		Permission: "approval_required",
	}, handleRestartContainer)

	// --- remove_container ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "remove_container",
		Description: "Remove a container (force=true removes a running container)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Container ID or name",
				},
				"force": map[string]any{
					"type":        "boolean",
					"description": "Force removal of a running container (default false)",
				},
			},
			"required": []string{"id"},
		},
		Permission: "approval_required",
	}, handleRemoveContainer)

	// --- pull_image ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "pull_image",
		Description: "Pull an image from a registry",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"image": map[string]any{
					"type":        "string",
					"description": "Image reference, e.g. nginx:latest",
				},
			},
			"required": []string{"image"},
		},
		Permission: "approval_required",
	}, handlePullImage)

	// --- run_container ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "run_container",
		Description: "Run a new detached container from an image",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"image": map[string]any{
					"type":        "string",
					"description": "Image reference, e.g. nginx:latest",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Container name (optional)",
				},
				"ports": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Port mappings, e.g. [\"8080:80\"] (optional)",
				},
				"env": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Environment variables, e.g. [\"KEY=value\"] (optional)",
				},
				"volumes": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Volume mounts, e.g. [\"/host:/container\"] (optional)",
				},
				"restart": map[string]any{
					"type":        "string",
					"description": "Restart policy, e.g. unless-stopped (optional)",
				},
			},
			"required": []string{"image"},
		},
		Permission: "approval_required",
	}, handleRunContainer)

	// --- compose_up ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "compose_up",
		Description: "Deploy a docker compose stack from YAML content",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"compose_yaml": map[string]any{
					"type":        "string",
					"description": "docker-compose YAML content",
				},
				"project_name": map[string]any{
					"type":        "string",
					"description": "Compose project name",
				},
			},
			"required": []string{"compose_yaml", "project_name"},
		},
		Permission: "approval_required",
	}, handleComposeUp)

	// --- compose_down ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "compose_down",
		Description: "Tear down a docker compose project",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_name": map[string]any{
					"type":        "string",
					"description": "Compose project name",
				},
			},
			"required": []string{"project_name"},
		},
		Permission: "approval_required",
	}, handleComposeDown)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig.String())
		server.Close()
		os.Exit(0)
	}()

	logger.Info("starting axios-docker MCP server", "socket", *socketPath)
	if err := server.Serve(*socketPath); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// --- Tool handlers ---
// Handler errors become IsError tool results in pkg/mcp, so a missing docker
// CLI surfaces as a clear error message instead of crashing the server.

// requireString extracts a required non-empty string parameter.
func requireString(params map[string]any, key string) (string, error) {
	v, ok := params[key].(string)
	if !ok || v == "" {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	return v, nil
}

// stringSlice extracts an optional array-of-strings parameter.
func stringSlice(params map[string]any, key string) ([]string, error) {
	v, ok := params[key]
	if !ok || v == nil {
		return nil, nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("parameter %s must be an array of strings", key)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("parameter %s must be an array of strings", key)
		}
		out = append(out, s)
	}
	return out, nil
}

func handleListContainers(params map[string]any) (string, error) {
	all, _ := params["all"].(bool)

	containers, err := dockerctl.ListContainers(all)
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		if all {
			return "no containers found", nil
		}
		return "no running containers (set all=true to include stopped ones)", nil
	}

	out, err := json.Marshal(containers)
	if err != nil {
		return "", fmt.Errorf("marshal containers: %w", err)
	}
	return fmt.Sprintf("%d containers:\n%s", len(containers), out), nil
}

func handleContainerInfo(params map[string]any) (string, error) {
	id, err := requireString(params, "id")
	if err != nil {
		return "", err
	}

	data, err := dockerctl.InspectContainer(id)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func handleContainerLogs(params map[string]any) (string, error) {
	id, err := requireString(params, "id")
	if err != nil {
		return "", err
	}

	tail := 100
	if t, ok := params["tail"].(float64); ok && int(t) > 0 {
		tail = int(t)
	}

	logs, err := dockerctl.ContainerLogs(id, tail)
	if err != nil {
		return "", err
	}
	if logs == "" {
		return fmt.Sprintf("no log output for container %s", id), nil
	}
	return logs, nil
}

func handleListImages(params map[string]any) (string, error) {
	images, err := dockerctl.ListImages()
	if err != nil {
		return "", err
	}
	if len(images) == 0 {
		return "no images found", nil
	}

	out, err := json.Marshal(images)
	if err != nil {
		return "", fmt.Errorf("marshal images: %w", err)
	}
	return fmt.Sprintf("%d images:\n%s", len(images), out), nil
}

func handleDockerStats(params map[string]any) (string, error) {
	stats, err := dockerctl.Stats()
	if err != nil {
		return "", err
	}
	if len(stats) == 0 {
		return "no running containers", nil
	}

	out, err := json.Marshal(stats)
	if err != nil {
		return "", fmt.Errorf("marshal stats: %w", err)
	}
	return fmt.Sprintf("stats for %d containers:\n%s", len(stats), out), nil
}

func handleStartContainer(params map[string]any) (string, error) {
	id, err := requireString(params, "id")
	if err != nil {
		return "", err
	}

	if err := dockerctl.StartContainer(id); err != nil {
		return "", err
	}
	return fmt.Sprintf("container %s started", id), nil
}

func handleStopContainer(params map[string]any) (string, error) {
	id, err := requireString(params, "id")
	if err != nil {
		return "", err
	}

	if err := dockerctl.StopContainer(id); err != nil {
		return "", err
	}
	return fmt.Sprintf("container %s stopped", id), nil
}

func handleRestartContainer(params map[string]any) (string, error) {
	id, err := requireString(params, "id")
	if err != nil {
		return "", err
	}

	if err := dockerctl.RestartContainer(id); err != nil {
		return "", err
	}
	return fmt.Sprintf("container %s restarted", id), nil
}

func handleRemoveContainer(params map[string]any) (string, error) {
	id, err := requireString(params, "id")
	if err != nil {
		return "", err
	}
	force, _ := params["force"].(bool)

	if err := dockerctl.RemoveContainer(id, force); err != nil {
		return "", err
	}
	return fmt.Sprintf("container %s removed", id), nil
}

func handlePullImage(params map[string]any) (string, error) {
	image, err := requireString(params, "image")
	if err != nil {
		return "", err
	}

	output, err := dockerctl.PullImage(image)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pulled %s:\n%s", image, output), nil
}

func handleRunContainer(params map[string]any) (string, error) {
	image, err := requireString(params, "image")
	if err != nil {
		return "", err
	}
	name, _ := params["name"].(string)
	restart, _ := params["restart"].(string)

	ports, err := stringSlice(params, "ports")
	if err != nil {
		return "", err
	}
	env, err := stringSlice(params, "env")
	if err != nil {
		return "", err
	}
	volumes, err := stringSlice(params, "volumes")
	if err != nil {
		return "", err
	}

	containerID, err := dockerctl.RunContainer(image, name, ports, env, volumes, restart)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("container started from %s: %s", image, containerID), nil
}

func handleComposeUp(params map[string]any) (string, error) {
	composeYaml, err := requireString(params, "compose_yaml")
	if err != nil {
		return "", err
	}
	projectName, err := requireString(params, "project_name")
	if err != nil {
		return "", err
	}

	output, err := dockerctl.ComposeUp(composeYaml, projectName)
	if err != nil {
		return "", err
	}
	if output == "" {
		return fmt.Sprintf("compose project %s is up", projectName), nil
	}
	return output, nil
}

func handleComposeDown(params map[string]any) (string, error) {
	projectName, err := requireString(params, "project_name")
	if err != nil {
		return "", err
	}

	output, err := dockerctl.ComposeDown(projectName)
	if err != nil {
		return "", err
	}
	if output == "" {
		return fmt.Sprintf("compose project %s is down", projectName), nil
	}
	return output, nil
}
