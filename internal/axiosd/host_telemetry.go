package axiosd

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultTelemetryPort = 3000

type RunningModelStats struct {
	Name      string `json:"name"`
	SizeBytes uint64 `json:"size_bytes"`
	VRAMBytes uint64 `json:"vram_bytes"`
}

type HostTelemetry struct {
	Host          *OllamaHost         `json:"host"`
	Source        string              `json:"source"`
	System        *SystemStats        `json:"system,omitempty"`
	OllamaVersion string              `json:"ollama_version,omitempty"`
	RunningModels []RunningModelStats `json:"running_models"`
	LatencyMS     int64               `json:"latency_ms"`
	Message       string              `json:"message,omitempty"`
}

func hostURL(host string, port int, path string) string {
	return "http://" + net.JoinHostPort(strings.Trim(host, "[]"), fmt.Sprintf("%d", port)) + path
}

func fetchJSON(client *http.Client, url string, target any, bearerToken string) error {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func collectHostTelemetry(host *OllamaHost) HostTelemetry {
	telemetry := HostTelemetry{
		Host:          host,
		Source:        "ollama",
		RunningModels: []RunningModelStats{},
	}
	client := &http.Client{Timeout: 2500 * time.Millisecond}

	started := time.Now()
	var versionResponse struct {
		Version string `json:"version"`
	}
	var processResponse struct {
		Models []struct {
			Name     string `json:"name"`
			Size     uint64 `json:"size"`
			SizeVRAM uint64 `json:"size_vram"`
		} `json:"models"`
	}
	var versionErr, processErr error
	telemetryPort := host.TelemetryPort
	if telemetryPort <= 0 {
		telemetryPort = defaultTelemetryPort
	}
	var systemStats SystemStats
	var systemErr error
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		versionErr = fetchJSON(client, hostURL(host.Host, host.Port, "/api/version"), &versionResponse, "")
	}()
	go func() {
		defer wait.Done()
		processErr = fetchJSON(client, hostURL(host.Host, host.Port, "/api/ps"), &processResponse, "")
	}()
	if host.ID != "local" {
		wait.Add(1)
		go func() {
			defer wait.Done()
			systemErr = fetchJSON(client, hostURL(host.Host, telemetryPort, "/api/system/stats"), &systemStats, host.TelemetryToken)
		}()
	}
	wait.Wait()
	telemetry.LatencyMS = time.Since(started).Milliseconds()
	if versionErr == nil {
		telemetry.OllamaVersion = versionResponse.Version
	}
	if processErr == nil {
		for _, model := range processResponse.Models {
			telemetry.RunningModels = append(telemetry.RunningModels, RunningModelStats{
				Name: model.Name, SizeBytes: model.Size, VRAMBytes: model.SizeVRAM,
			})
		}
	}

	if host.ID == "local" {
		telemetry.Source = "local"
		telemetry.System, _ = gatherSystemStats()
		return telemetry
	}

	if systemErr == nil {
		telemetry.Source = "agent"
		telemetry.System = &systemStats
	} else {
		if host.TelemetryToken == "" {
			telemetry.Message = fmt.Sprintf("A telemetry token is required for AxiOS on port %d", telemetryPort)
		} else {
			telemetry.Message = fmt.Sprintf("Authenticated hardware telemetry is unavailable on port %d", telemetryPort)
		}
	}
	return telemetry
}

func (s *Server) handleHostStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.hostStore == nil {
		s.jsonError(w, "host management not initialized", http.StatusServiceUnavailable)
		return
	}

	var host *OllamaHost
	if id := r.URL.Query().Get("id"); id != "" {
		for _, candidate := range s.hostStore.GetHosts() {
			if candidate.ID == id {
				host = candidate
				break
			}
		}
	} else {
		host = s.hostStore.GetActive()
	}
	if host == nil {
		s.jsonError(w, "host not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(collectHostTelemetry(host))
}
