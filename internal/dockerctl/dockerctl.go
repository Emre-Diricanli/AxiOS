// Package dockerctl wraps the docker CLI for container, image, compose, and
// stats operations. It is shared by the axiosd REST handlers and the
// axios-docker MCP server so Docker logic lives in one place.
package dockerctl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Command execution is injected behind package-level function variables so
// tests can fake docker CLI output without a real daemon.
var (
	// lookDocker reports whether the docker CLI is on PATH.
	lookDocker = func() error {
		_, err := exec.LookPath("docker")
		return err
	}
	// runDocker executes `docker` with args and returns combined stdout+stderr.
	runDocker = func(args ...string) ([]byte, error) {
		return exec.Command("docker", args...).CombinedOutput()
	}
)

// --- Docker data types ---

// DockerContainer represents a container from `docker ps`.
type DockerContainer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	State   string `json:"state"`
	Ports   string `json:"ports"`
	Created string `json:"created"`
}

// DockerImage represents an image from `docker images`.
type DockerImage struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	Created    string `json:"created"`
}

// DockerContainerStats represents resource usage from `docker stats`.
type DockerContainerStats struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	CPUPerc  string `json:"cpu_perc"`
	MemUsage string `json:"mem_usage"`
	MemPerc  string `json:"mem_perc"`
	NetIO    string `json:"net_io"`
	BlockIO  string `json:"block_io"`
	PIDs     string `json:"pids"`
}

// --- Docker CLI operations ---

// Available checks whether the docker CLI is installed and reachable.
func Available() error {
	if err := lookDocker(); err != nil {
		return fmt.Errorf("docker CLI not found in PATH: %w", err)
	}
	return nil
}

// ListContainers runs `docker ps --format json` (with -a when all is true)
// and parses the output.
func ListContainers(all bool) ([]DockerContainer, error) {
	if err := Available(); err != nil {
		return nil, err
	}

	args := []string{"ps"}
	if all {
		args = append(args, "-a")
	}
	args = append(args, "--format", "json")

	out, err := runDocker(args...)
	if err != nil {
		return nil, fmt.Errorf("docker ps failed: %s", strings.TrimSpace(string(out)))
	}

	var containers []DockerContainer
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		c := DockerContainer{
			ID:      strVal(raw, "ID"),
			Name:    strVal(raw, "Names"),
			Image:   strVal(raw, "Image"),
			Status:  strVal(raw, "Status"),
			State:   strVal(raw, "State"),
			Ports:   strVal(raw, "Ports"),
			Created: strVal(raw, "CreatedAt"),
		}
		containers = append(containers, c)
	}

	if containers == nil {
		containers = []DockerContainer{}
	}
	return containers, nil
}

// InspectContainer runs `docker inspect <id>` and returns the parsed JSON.
func InspectContainer(id string) (json.RawMessage, error) {
	if err := Available(); err != nil {
		return nil, err
	}

	out, err := runDocker("inspect", id)
	if err != nil {
		return nil, fmt.Errorf("docker inspect failed: %s", strings.TrimSpace(string(out)))
	}

	// Validate that the output is valid JSON.
	var parsed json.RawMessage
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse inspect output: %w", err)
	}
	return parsed, nil
}

// ContainerLogs runs `docker logs --tail <tail> <id>` and returns stdout+stderr.
func ContainerLogs(id string, tail int) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}

	out, err := runDocker("logs", "--tail", strconv.Itoa(tail), id)
	if err != nil {
		return "", fmt.Errorf("docker logs failed: %s", strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// StartContainer runs `docker start <id>`.
func StartContainer(id string) error {
	if err := Available(); err != nil {
		return err
	}

	out, err := runDocker("start", id)
	if err != nil {
		return fmt.Errorf("docker start failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// StopContainer runs `docker stop <id>`.
func StopContainer(id string) error {
	if err := Available(); err != nil {
		return err
	}

	out, err := runDocker("stop", id)
	if err != nil {
		return fmt.Errorf("docker stop failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// RestartContainer runs `docker restart <id>`.
func RestartContainer(id string) error {
	if err := Available(); err != nil {
		return err
	}

	out, err := runDocker("restart", id)
	if err != nil {
		return fmt.Errorf("docker restart failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveContainer runs `docker rm <id>` (with -f if force is true).
func RemoveContainer(id string, force bool) error {
	if err := Available(); err != nil {
		return err
	}

	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, id)

	out, err := runDocker(args...)
	if err != nil {
		return fmt.Errorf("docker rm failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// PullImage runs `docker pull <image>`.
func PullImage(image string) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}

	out, err := runDocker("pull", image)
	if err != nil {
		return "", fmt.Errorf("docker pull failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// RunContainer runs `docker run -d` with the given options and returns the
// new container ID.
func RunContainer(image, name string, ports, env, volumes []string, restart string) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}

	args := []string{"run", "-d"}

	if name != "" {
		args = append(args, "--name", name)
	}
	for _, p := range ports {
		args = append(args, "-p", p)
	}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	for _, v := range volumes {
		args = append(args, "-v", v)
	}
	if restart != "" {
		args = append(args, "--restart", restart)
	}
	args = append(args, image)

	out, err := runDocker(args...)
	if err != nil {
		return "", fmt.Errorf("docker run failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// ListImages runs `docker images --format json` and parses the output.
func ListImages() ([]DockerImage, error) {
	if err := Available(); err != nil {
		return nil, err
	}

	out, err := runDocker("images", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("docker images failed: %s", strings.TrimSpace(string(out)))
	}

	var images []DockerImage
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		img := DockerImage{
			ID:         strVal(raw, "ID"),
			Repository: strVal(raw, "Repository"),
			Tag:        strVal(raw, "Tag"),
			Size:       strVal(raw, "Size"),
			Created:    strVal(raw, "CreatedAt"),
		}
		images = append(images, img)
	}

	if images == nil {
		images = []DockerImage{}
	}
	return images, nil
}

// ComposeUp writes YAML to a temp file and runs
// `docker compose -f <file> -p <projectName> up -d`.
func ComposeUp(composeYaml, projectName string) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}

	tmpFile, err := os.CreateTemp("", "axios-compose-*.yml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(composeYaml); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write compose YAML: %w", err)
	}
	tmpFile.Close()

	args := []string{"compose", "-f", tmpFile.Name()}
	if projectName != "" {
		args = append(args, "-p", projectName)
	}
	args = append(args, "up", "-d")

	out, err := runDocker(args...)
	if err != nil {
		return "", fmt.Errorf("docker compose up failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// ComposeDown runs `docker compose -p <projectName> down`.
func ComposeDown(projectName string) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}

	args := []string{"compose"}
	if projectName != "" {
		args = append(args, "-p", projectName)
	}
	args = append(args, "down")

	out, err := runDocker(args...)
	if err != nil {
		return "", fmt.Errorf("docker compose down failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Stats runs `docker stats --no-stream --format json` and returns resource usage.
func Stats() ([]DockerContainerStats, error) {
	if err := Available(); err != nil {
		return nil, err
	}

	out, err := runDocker("stats", "--no-stream", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("docker stats failed: %s", strings.TrimSpace(string(out)))
	}

	var stats []DockerContainerStats
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		st := DockerContainerStats{
			ID:       strVal(raw, "ID"),
			Name:     strVal(raw, "Name"),
			CPUPerc:  strVal(raw, "CPUPerc"),
			MemUsage: strVal(raw, "MemUsage"),
			MemPerc:  strVal(raw, "MemPerc"),
			NetIO:    strVal(raw, "NetIO"),
			BlockIO:  strVal(raw, "BlockIO"),
			PIDs:     strVal(raw, "PIDs"),
		}
		stats = append(stats, st)
	}

	if stats == nil {
		stats = []DockerContainerStats{}
	}
	return stats, nil
}

// strVal safely extracts a string value from a map.
func strVal(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}
