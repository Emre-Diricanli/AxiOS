package axiosd

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/axios-os/axios/internal/dockerctl"
)

// --- HTTP handler methods ---
// Docker CLI logic lives in internal/dockerctl, shared with the axios-docker
// MCP server. These handlers only adapt HTTP requests to that package.

// handleDockerContainers handles GET (list) and POST (run new) on /api/docker/containers.
// DELETE with ?id=X&force=true also removes a container.
func (s *Server) handleDockerContainers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		containers, err := dockerctl.ListContainers(true)
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

		containerID, err := dockerctl.RunContainer(req.Image, req.Name, req.Ports, req.Env, req.Volumes, req.Restart)
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

		if err := dockerctl.RemoveContainer(id, force); err != nil {
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

	data, err := dockerctl.InspectContainer(id)
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
		err = dockerctl.StartContainer(id)
	case "stop":
		err = dockerctl.StopContainer(id)
	case "restart":
		err = dockerctl.RestartContainer(id)
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

	logs, err := dockerctl.ContainerLogs(id, tail)
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

	images, err := dockerctl.ListImages()
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

	output, err := dockerctl.PullImage(req.Image)
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
		output, err = dockerctl.ComposeUp(req.YAML, req.Project)
	case "down":
		if req.Project == "" {
			s.jsonError(w, "project is required for compose down", http.StatusBadRequest)
			return
		}
		output, err = dockerctl.ComposeDown(req.Project)
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

	stats, err := dockerctl.Stats()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"stats": stats})
}
