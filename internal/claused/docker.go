package claused

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

// --- Docker CLI helper functions ---

// dockerAvailable checks whether the docker CLI is installed and reachable.
func dockerAvailable() error {
	_, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker CLI not found in PATH: %w", err)
	}
	return nil
}

// listContainers runs `docker ps -a --format json` and parses the output.
func listContainers() ([]DockerContainer, error) {
	if err := dockerAvailable(); err != nil {
		return nil, err
	}

	out, err := exec.Command("docker", "ps", "-a", "--format", "json").CombinedOutput()
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

// inspectContainer runs `docker inspect <id>` and returns the parsed JSON.
func inspectContainer(id string) (json.RawMessage, error) {
	if err := dockerAvailable(); err != nil {
		return nil, err
	}

	out, err := exec.Command("docker", "inspect", id).CombinedOutput()
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

// containerLogs runs `docker logs --tail <tail> <id>` and returns stdout+stderr.
func containerLogs(id string, tail int) (string, error) {
	if err := dockerAvailable(); err != nil {
		return "", err
	}

	tailStr := strconv.Itoa(tail)
	out, err := exec.Command("docker", "logs", "--tail", tailStr, id).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker logs failed: %s", strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// startContainer runs `docker start <id>`.
func startContainer(id string) error {
	if err := dockerAvailable(); err != nil {
		return err
	}

	out, err := exec.Command("docker", "start", id).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker start failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// stopContainer runs `docker stop <id>`.
func stopContainer(id string) error {
	if err := dockerAvailable(); err != nil {
		return err
	}

	out, err := exec.Command("docker", "stop", id).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stop failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// restartContainer runs `docker restart <id>`.
func restartContainer(id string) error {
	if err := dockerAvailable(); err != nil {
		return err
	}

	out, err := exec.Command("docker", "restart", id).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker restart failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// removeContainer runs `docker rm <id>` (with -f if force is true).
func removeContainer(id string, force bool) error {
	if err := dockerAvailable(); err != nil {
		return err
	}

	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, id)

	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker rm failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// pullImage runs `docker pull <image>`.
func pullImage(image string) (string, error) {
	if err := dockerAvailable(); err != nil {
		return "", err
	}

	out, err := exec.Command("docker", "pull", image).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker pull failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// runContainer runs `docker run -d` with the given options.
func runContainer(image, name string, ports []string, env []string, volumes []string, restart string) (string, error) {
	if err := dockerAvailable(); err != nil {
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

	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// listImages runs `docker images --format json` and parses the output.
func listImages() ([]DockerImage, error) {
	if err := dockerAvailable(); err != nil {
		return nil, err
	}

	out, err := exec.Command("docker", "images", "--format", "json").CombinedOutput()
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

// composeUp writes YAML to a temp file and runs `docker compose -f <file> -p <projectName> up -d`.
func composeUp(composeYaml string, projectName string) (string, error) {
	if err := dockerAvailable(); err != nil {
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

	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker compose up failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// composeDown runs `docker compose -p <projectName> down`.
func composeDown(projectName string) (string, error) {
	if err := dockerAvailable(); err != nil {
		return "", err
	}

	args := []string{"compose"}
	if projectName != "" {
		args = append(args, "-p", projectName)
	}
	args = append(args, "down")

	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker compose down failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// dockerStats runs `docker stats --no-stream --format json` and returns resource usage.
func dockerStats() ([]DockerContainerStats, error) {
	if err := dockerAvailable(); err != nil {
		return nil, err
	}

	out, err := exec.Command("docker", "stats", "--no-stream", "--format", "json").CombinedOutput()
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

// --- HTTP handler methods ---

// handleDockerContainers handles GET (list) and POST (run new) on /api/docker/containers.
// DELETE with ?id=X&force=true also removes a container.
func (s *Server) handleDockerContainers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		containers, err := listContainers()
		if err != nil {
			s.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"containers": containers})

	case http.MethodPost:
		var req struct {
			Image   string   `json:"image"`
			Name    string   `json:"name"`
			Ports   []string `json:"ports"`
			Env     []string `json:"env"`
			Volumes []string `json:"volumes"`
			Restart string   `json:"restart"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if req.Image == "" {
			s.jsonError(w, "image is required", http.StatusBadRequest)
			return
		}

		containerID, err := runContainer(req.Image, req.Name, req.Ports, req.Env, req.Volumes, req.Restart)
		if err != nil {
			s.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "container_id": containerID})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			s.jsonError(w, "id query parameter required", http.StatusBadRequest)
			return
		}
		force := r.URL.Query().Get("force") == "true"

		if err := removeContainer(id, force); err != nil {
			s.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})

	default:
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDockerContainer handles GET (inspect) on /api/docker/containers/inspect?id=X.
func (s *Server) handleDockerContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		s.jsonError(w, "id query parameter required", http.StatusBadRequest)
		return
	}

	data, err := inspectContainer(id)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// handleDockerContainerAction handles POST on /api/docker/containers/action?id=X&action=start|stop|restart.
func (s *Server) handleDockerContainerAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		s.jsonError(w, "id query parameter required", http.StatusBadRequest)
		return
	}

	action := r.URL.Query().Get("action")
	var err error
	switch action {
	case "start":
		err = startContainer(id)
	case "stop":
		err = stopContainer(id)
	case "restart":
		err = restartContainer(id)
	default:
		s.jsonError(w, "action must be start, stop, or restart", http.StatusBadRequest)
		return
	}

	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "action": action, "id": id})
}

// handleDockerContainerLogs handles GET on /api/docker/containers/logs?id=X&tail=100.
func (s *Server) handleDockerContainerLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		s.jsonError(w, "id query parameter required", http.StatusBadRequest)
		return
	}

	tail := 100
	if t := r.URL.Query().Get("tail"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil && parsed > 0 {
			tail = parsed
		}
	}

	logs, err := containerLogs(id, tail)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"logs": logs})
}

// handleDockerImages handles GET (list) on /api/docker/images.
func (s *Server) handleDockerImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	images, err := listImages()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"images": images})
}

// handleDockerImagePull handles POST on /api/docker/images/pull.
func (s *Server) handleDockerImagePull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Image string `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.Image == "" {
		s.jsonError(w, "image is required", http.StatusBadRequest)
		return
	}

	output, err := pullImage(req.Image)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "output": output})
}

// handleDockerCompose handles POST on /api/docker/compose for up/down operations.
func (s *Server) handleDockerCompose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		YAML    string `json:"yaml"`
		Project string `json:"project"`
		Action  string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	var output string
	var err error

	switch req.Action {
	case "up":
		if req.YAML == "" {
			s.jsonError(w, "yaml is required for compose up", http.StatusBadRequest)
			return
		}
		output, err = composeUp(req.YAML, req.Project)
	case "down":
		if req.Project == "" {
			s.jsonError(w, "project is required for compose down", http.StatusBadRequest)
			return
		}
		output, err = composeDown(req.Project)
	default:
		s.jsonError(w, "action must be up or down", http.StatusBadRequest)
		return
	}

	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "output": output})
}

// handleDockerStats handles GET on /api/docker/stats.
func (s *Server) handleDockerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := dockerStats()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"stats": stats})
}
