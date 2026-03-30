package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/axios-os/axios/pkg/logging"
	"github.com/axios-os/axios/pkg/mcp"
)

func main() {
	socketPath := flag.String("socket", mcp.SocketPath("axios-system"), "Unix socket path")
	flag.Parse()

	logger := logging.New("axios-system")

	server := mcp.NewServer("axios-system", "0.1.0")

	// --- run_command ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "run_command",
		Description: "Execute a bash command and return stdout+stderr",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The bash command to execute",
				},
				"working_dir": map[string]any{
					"type":        "string",
					"description": "Working directory for the command (optional)",
				},
			},
			"required": []string{"command"},
		},
		Permission: "trusted",
	}, handleRunCommand)

	// --- system_info ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "system_info",
		Description: "Returns CPU, memory, hostname, OS, and kernel information",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "trusted",
	}, handleSystemInfo)

	// --- disk_usage ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "disk_usage",
		Description: "Returns disk usage for mounted filesystems",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "trusted",
	}, handleDiskUsage)

	// --- process_list ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "process_list",
		Description: "Lists running processes",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "trusted",
	}, handleProcessList)

	// --- service_status ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "service_status",
		Description: "Check systemd service status",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"service": map[string]any{
					"type":        "string",
					"description": "Name of the systemd service",
				},
			},
			"required": []string{"service"},
		},
		Permission: "trusted",
	}, handleServiceStatus)

	// --- reboot ---
	server.RegisterTool(mcp.ToolDefinition{
		Name:        "reboot",
		Description: "Reboot the system",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Permission: "approval_required",
	}, handleReboot)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig.String())
		server.Close()
		os.Exit(0)
	}()

	logger.Info("starting axios-system MCP server", "socket", *socketPath)
	if err := server.Serve(*socketPath); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// handleRunCommand executes a bash command with a 30-second timeout.
func handleRunCommand(params map[string]any) (string, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("missing required parameter: command")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)

	if workingDir, ok := params["working_dir"].(string); ok && workingDir != "" {
		cmd.Dir = workingDir
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()

	result := output.String()
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("command timed out after 30 seconds")
	}
	if err != nil {
		return fmt.Sprintf("%s\nexit status: %s", result, err.Error()), nil
	}

	return result, nil
}

// handleSystemInfo gathers CPU, memory, hostname, OS, and kernel info.
func handleSystemInfo(params map[string]any) (string, error) {
	var info strings.Builder

	// Hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	fmt.Fprintf(&info, "Hostname: %s\n", hostname)

	// OS and Architecture
	fmt.Fprintf(&info, "OS: %s\n", runtime.GOOS)
	fmt.Fprintf(&info, "Architecture: %s\n", runtime.GOARCH)
	fmt.Fprintf(&info, "Go CPUs (logical): %d\n", runtime.NumCPU())

	// Kernel version
	kernelVersion := getKernelVersion()
	fmt.Fprintf(&info, "Kernel: %s\n", kernelVersion)

	// CPU info
	cpuInfo := getCPUInfo()
	fmt.Fprintf(&info, "\n--- CPU ---\n%s\n", cpuInfo)

	// Memory info
	memInfo := getMemInfo()
	fmt.Fprintf(&info, "\n--- Memory ---\n%s\n", memInfo)

	return info.String(), nil
}

// getKernelVersion returns the kernel version string.
func getKernelVersion() string {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// getCPUInfo returns CPU information, with platform-specific fallbacks.
func getCPUInfo() string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/cpuinfo")
		if err == nil {
			// Extract model name and count cores
			lines := strings.Split(string(data), "\n")
			modelName := ""
			coreCount := 0
			for _, line := range lines {
				if strings.HasPrefix(line, "model name") {
					coreCount++
					if modelName == "" {
						parts := strings.SplitN(line, ":", 2)
						if len(parts) == 2 {
							modelName = strings.TrimSpace(parts[1])
						}
					}
				}
			}
			if modelName != "" {
				return fmt.Sprintf("Model: %s\nCores: %d", modelName, coreCount)
			}
			return string(data)
		}
	}

	// macOS fallback using sysctl
	if runtime.GOOS == "darwin" {
		var info strings.Builder

		brand, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil {
			fmt.Fprintf(&info, "Model: %s\n", strings.TrimSpace(string(brand)))
		}

		coreCount, err := exec.Command("sysctl", "-n", "hw.physicalcpu").Output()
		if err == nil {
			fmt.Fprintf(&info, "Physical Cores: %s\n", strings.TrimSpace(string(coreCount)))
		}

		logicalCount, err := exec.Command("sysctl", "-n", "hw.logicalcpu").Output()
		if err == nil {
			fmt.Fprintf(&info, "Logical Cores: %s\n", strings.TrimSpace(string(logicalCount)))
		}

		if info.Len() > 0 {
			return info.String()
		}
	}

	return fmt.Sprintf("Logical CPUs: %d", runtime.NumCPU())
}

// getMemInfo returns memory information, with platform-specific fallbacks.
func getMemInfo() string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			// Extract key memory stats
			lines := strings.Split(string(data), "\n")
			var result strings.Builder
			for _, line := range lines {
				if strings.HasPrefix(line, "MemTotal:") ||
					strings.HasPrefix(line, "MemFree:") ||
					strings.HasPrefix(line, "MemAvailable:") ||
					strings.HasPrefix(line, "SwapTotal:") ||
					strings.HasPrefix(line, "SwapFree:") {
					result.WriteString(line)
					result.WriteString("\n")
				}
			}
			if result.Len() > 0 {
				return result.String()
			}
			return string(data)
		}
	}

	// macOS fallback using sysctl
	if runtime.GOOS == "darwin" {
		var info strings.Builder

		memSize, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err == nil {
			memStr := strings.TrimSpace(string(memSize))
			fmt.Fprintf(&info, "Total Memory (bytes): %s\n", memStr)
		}

		// Get memory pressure summary via vm_stat
		vmStat, err := exec.Command("vm_stat").Output()
		if err == nil {
			fmt.Fprintf(&info, "\n%s", string(vmStat))
		}

		if info.Len() > 0 {
			return info.String()
		}
	}

	return "memory info not available"
}

// handleDiskUsage runs df -h and returns the output.
func handleDiskUsage(params map[string]any) (string, error) {
	out, err := exec.Command("df", "-h").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get disk usage: %w", err)
	}
	return string(out), nil
}

// handleProcessList runs ps aux and returns the output.
func handleProcessList(params map[string]any) (string, error) {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return "", fmt.Errorf("failed to list processes: %w", err)
	}
	return string(out), nil
}

// handleServiceStatus checks systemd service status.
func handleServiceStatus(params map[string]any) (string, error) {
	service, ok := params["service"].(string)
	if !ok || service == "" {
		return "", fmt.Errorf("missing required parameter: service")
	}

	cmd := exec.Command("systemctl", "status", service)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	_ = cmd.Run() // systemctl returns non-zero for inactive/failed services
	return output.String(), nil
}

// handleReboot initiates a system reboot.
func handleReboot(params map[string]any) (string, error) {
	cmd := exec.Command("systemctl", "reboot")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		// Fallback to shutdown -r for non-systemd systems
		cmd = exec.Command("shutdown", "-r", "now")
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err := cmd.Run(); err != nil {
			return output.String(), fmt.Errorf("failed to reboot: %w", err)
		}
	}

	return "reboot initiated", nil
}
