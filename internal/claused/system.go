package claused

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// SystemStats contains live system statistics.
type SystemStats struct {
	Hostname string      `json:"hostname"`
	OS       string      `json:"os"`
	Arch     string      `json:"arch"`
	Kernel   string      `json:"kernel"`
	Uptime   string      `json:"uptime"`
	CPU      CPUStats    `json:"cpu"`
	Memory   MemStats    `json:"memory"`
	Disk     []DiskStats `json:"disk"`
	Network  NetStats    `json:"network"`
}

// CPUStats holds CPU information.
type CPUStats struct {
	Model        string  `json:"model"`
	Cores        int     `json:"cores"`
	Threads      int     `json:"threads"`
	UsagePercent float64 `json:"usage_percent"`
}

// MemStats holds memory information.
type MemStats struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
}

// DiskStats holds disk usage for a single mount point.
type DiskStats struct {
	Mount          string  `json:"mount"`
	Device         string  `json:"device"`
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
}

// NetStats holds network information.
type NetStats struct {
	Hostname   string         `json:"hostname"`
	Interfaces []NetInterface `json:"interfaces"`
}

// NetInterface holds info about one network interface.
type NetInterface struct {
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Status string `json:"status"`
}

// gatherSystemStats collects live system statistics.
func gatherSystemStats() (*SystemStats, error) {
	hostname, _ := os.Hostname()

	stats := &SystemStats{
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Kernel:   getKernel(),
		Uptime:   getUptime(),
		CPU:      getCPUStats(),
		Memory:   getMemoryStats(),
		Disk:     getDiskStats(),
		Network:  getNetworkStats(hostname),
	}

	return stats, nil
}

// getKernel returns the kernel version string.
func getKernel() string {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// getUptime returns a human-readable uptime string.
func getUptime() string {
	switch runtime.GOOS {
	case "linux":
		return getUptimeLinux()
	case "darwin":
		return getUptimeDarwin()
	default:
		return "unknown"
	}
}

func getUptimeLinux() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "unknown"
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return "unknown"
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "unknown"
	}
	return formatDuration(time.Duration(seconds) * time.Second)
}

func getUptimeDarwin() string {
	out, err := exec.Command("sysctl", "-n", "kern.boottime").Output()
	if err != nil {
		return "unknown"
	}
	// Output format: { sec = 1234567890, usec = 123456 } ...
	s := string(out)
	idx := strings.Index(s, "sec = ")
	if idx == -1 {
		return "unknown"
	}
	s = s[idx+6:]
	endIdx := strings.Index(s, ",")
	if endIdx == -1 {
		return "unknown"
	}
	bootSec, err := strconv.ParseInt(s[:endIdx], 10, 64)
	if err != nil {
		return "unknown"
	}
	bootTime := time.Unix(bootSec, 0)
	return formatDuration(time.Since(bootTime))
}

// formatDuration formats a duration as "Xd Yh Zm".
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// getCPUStats returns CPU model, core count, and usage.
func getCPUStats() CPUStats {
	cores := runtime.NumCPU()
	stats := CPUStats{
		Model:   getCPUModel(),
		Cores:   cores,
		Threads: cores, // Go's NumCPU returns logical CPUs (threads)
	}

	switch runtime.GOOS {
	case "linux":
		stats.Cores = getCPUCoresLinux()
		stats.UsagePercent = getCPUUsageLinux()
	case "darwin":
		stats.Cores = getCPUCoresDarwin()
		stats.UsagePercent = getCPUUsageDarwin()
	}

	return stats
}

func getCPUModel() string {
	switch runtime.GOOS {
	case "linux":
		return getCPUModelLinux()
	case "darwin":
		return getCPUModelDarwin()
	default:
		return "unknown"
	}
}

func getCPUModelLinux() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}

func getCPUModelDarwin() string {
	out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func getCPUCoresLinux() int {
	// Count physical cores from /proc/cpuinfo
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return runtime.NumCPU()
	}
	defer f.Close()

	coreIDs := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "core id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				coreIDs[strings.TrimSpace(parts[1])] = true
			}
		}
	}
	if len(coreIDs) > 0 {
		return len(coreIDs)
	}
	return runtime.NumCPU()
}

func getCPUCoresDarwin() int {
	out, err := exec.Command("sysctl", "-n", "hw.physicalcpu").Output()
	if err != nil {
		return runtime.NumCPU()
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return runtime.NumCPU()
	}
	return n
}

func getCPUUsageLinux() float64 {
	// Read /proc/stat twice 1 second apart
	read := func() (idle, total uint64) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) == 0 {
			return 0, 0
		}
		fields := strings.Fields(lines[0]) // "cpu" aggregate line
		if len(fields) < 5 {
			return 0, 0
		}
		var vals []uint64
		for _, f := range fields[1:] {
			v, _ := strconv.ParseUint(f, 10, 64)
			vals = append(vals, v)
		}
		var sum uint64
		for _, v := range vals {
			sum += v
		}
		idleVal := vals[3] // 4th field is idle
		return idleVal, sum
	}

	idle1, total1 := read()
	time.Sleep(500 * time.Millisecond)
	idle2, total2 := read()

	if total2 <= total1 {
		return 0
	}

	totalDelta := float64(total2 - total1)
	idleDelta := float64(idle2 - idle1)
	usage := (1.0 - idleDelta/totalDelta) * 100.0
	if usage < 0 {
		return 0
	}
	return math.Round(usage*10) / 10
}

func getCPUUsageDarwin() float64 {
	out, err := exec.Command("top", "-l", "1", "-n", "0", "-s", "0").Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "CPU usage:") {
			// Format: CPU usage: 12.34% user, 5.67% sys, 81.99% idle
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "idle" && i > 0 {
					idleStr := strings.TrimSuffix(parts[i-1], "%")
					idle, err := strconv.ParseFloat(idleStr, 64)
					if err != nil {
						return 0
					}
					usage := 100.0 - idle
					return math.Round(usage*10) / 10
				}
			}
		}
	}
	return 0
}

// getMemoryStats returns memory usage information.
func getMemoryStats() MemStats {
	switch runtime.GOOS {
	case "linux":
		return getMemoryLinux()
	case "darwin":
		return getMemoryDarwin()
	default:
		return MemStats{}
	}
}

func getMemoryLinux() MemStats {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemStats{}
	}
	defer f.Close()

	info := make(map[string]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		val, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}
		info[key] = val * 1024 // Convert kB to bytes
	}

	total := info["MemTotal"]
	available := info["MemAvailable"]
	used := total - available
	var pct float64
	if total > 0 {
		pct = math.Round(float64(used)/float64(total)*1000) / 10
	}

	return MemStats{
		TotalBytes:     total,
		UsedBytes:      used,
		AvailableBytes: available,
		UsagePercent:   pct,
	}
}

func getMemoryDarwin() MemStats {
	// Get total memory
	totalOut, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return MemStats{}
	}
	total, err := strconv.ParseUint(strings.TrimSpace(string(totalOut)), 10, 64)
	if err != nil {
		return MemStats{}
	}

	// Get page size and vm_stat
	pageSizeOut, err := exec.Command("sysctl", "-n", "hw.pagesize").Output()
	if err != nil {
		return MemStats{TotalBytes: total}
	}
	pageSize, err := strconv.ParseUint(strings.TrimSpace(string(pageSizeOut)), 10, 64)
	if err != nil {
		return MemStats{TotalBytes: total}
	}

	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return MemStats{TotalBytes: total}
	}

	vmStats := make(map[string]uint64)
	for _, line := range strings.Split(string(vmOut), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, ".")
		val, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}
		vmStats[key] = val
	}

	// Calculate used memory (active + wired + compressed)
	active := vmStats["Pages active"] * pageSize
	wired := vmStats["Pages wired down"] * pageSize
	compressed := vmStats["Pages occupied by compressor"] * pageSize
	used := active + wired + compressed
	available := total - used

	var pct float64
	if total > 0 {
		pct = math.Round(float64(used)/float64(total)*1000) / 10
	}

	return MemStats{
		TotalBytes:     total,
		UsedBytes:      used,
		AvailableBytes: available,
		UsagePercent:   pct,
	}
}

// getDiskStats returns disk usage for all mounted filesystems.
func getDiskStats() []DiskStats {
	switch runtime.GOOS {
	case "linux":
		return getDiskLinux()
	case "darwin":
		return getDiskDarwin()
	default:
		return nil
	}
}

func getDiskLinux() []DiskStats {
	out, err := exec.Command("df", "-B1", "--output=source,target,size,used,avail").Output()
	if err != nil {
		// Fallback to regular df
		return parseDfOutput("df", "-B1")
	}
	return parseDfCustomOutput(string(out))
}

func parseDfCustomOutput(output string) []DiskStats {
	var disks []DiskStats
	lines := strings.Split(output, "\n")
	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		device := fields[0]
		mount := fields[1]

		// Skip virtual/temp filesystems
		if !strings.HasPrefix(device, "/dev/") {
			continue
		}

		totalB, _ := strconv.ParseUint(fields[2], 10, 64)
		usedB, _ := strconv.ParseUint(fields[3], 10, 64)
		availB, _ := strconv.ParseUint(fields[4], 10, 64)

		var pct float64
		if totalB > 0 {
			pct = math.Round(float64(usedB)/float64(totalB)*1000) / 10
		}

		disks = append(disks, DiskStats{
			Mount:          mount,
			Device:         device,
			TotalBytes:     totalB,
			UsedBytes:      usedB,
			AvailableBytes: availB,
			UsagePercent:   pct,
		})
	}
	return disks
}

func parseDfOutput(name string, args ...string) []DiskStats {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return nil
	}
	var disks []DiskStats
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		device := fields[0]
		if !strings.HasPrefix(device, "/dev/") {
			continue
		}

		// df output: Filesystem 1K-blocks Used Available Use% Mounted
		totalK, _ := strconv.ParseUint(fields[1], 10, 64)
		usedK, _ := strconv.ParseUint(fields[2], 10, 64)
		availK, _ := strconv.ParseUint(fields[3], 10, 64)
		mount := fields[5]
		if len(fields) > 6 {
			// Mount point might contain spaces (unlikely on Linux but handle gracefully)
			mount = strings.Join(fields[5:], " ")
		}

		totalB := totalK * 1024
		usedB := usedK * 1024
		availB := availK * 1024

		var pct float64
		if totalB > 0 {
			pct = math.Round(float64(usedB)/float64(totalB)*1000) / 10
		}

		disks = append(disks, DiskStats{
			Mount:          mount,
			Device:         device,
			TotalBytes:     totalB,
			UsedBytes:      usedB,
			AvailableBytes: availB,
			UsagePercent:   pct,
		})
	}
	return disks
}

func getDiskDarwin() []DiskStats {
	// macOS df -k outputs 512-byte blocks by default; -k gives 1K blocks
	out, err := exec.Command("df", "-k").Output()
	if err != nil {
		return nil
	}

	var disks []DiskStats
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] { // Skip header
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		device := fields[0]
		if !strings.HasPrefix(device, "/dev/") {
			continue
		}

		// macOS df -k: Filesystem 1024-blocks Used Available Capacity iused ifree %iused Mounted
		totalK, _ := strconv.ParseUint(fields[1], 10, 64)
		usedK, _ := strconv.ParseUint(fields[2], 10, 64)
		availK, _ := strconv.ParseUint(fields[3], 10, 64)
		mount := strings.Join(fields[8:], " ")

		// Skip macOS system partitions that clutter the output
		if strings.HasPrefix(mount, "/System/Volumes/") && mount != "/System/Volumes/Data" {
			continue
		}
		if mount == "/System/Volumes/Data" {
			// This is the same as /, skip the duplicate
			continue
		}
		// Skip tiny partitions (< 1GB) like Preboot, Recovery, Hardware
		if totalK < 1024*1024 {
			continue
		}

		totalB := totalK * 1024
		usedB := usedK * 1024
		availB := availK * 1024

		var pct float64
		if totalB > 0 {
			pct = math.Round(float64(usedB)/float64(totalB)*1000) / 10
		}

		disks = append(disks, DiskStats{
			Mount:          mount,
			Device:         device,
			TotalBytes:     totalB,
			UsedBytes:      usedB,
			AvailableBytes: availB,
			UsagePercent:   pct,
		})
	}
	return disks
}

// getNetworkStats returns information about network interfaces.
func getNetworkStats(hostname string) NetStats {
	ifaces, err := net.Interfaces()
	if err != nil {
		return NetStats{Hostname: hostname}
	}

	var interfaces []NetInterface
	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		status := "down"
		if iface.Flags&net.FlagUp != 0 {
			status = "up"
		}

		// Only include interfaces with an IPv4 address
		ip := ""
		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
					ip = ipNet.IP.String()
					break
				}
			}
		}
		if ip == "" {
			continue
		}

		interfaces = append(interfaces, NetInterface{
			Name:   iface.Name,
			IP:     ip,
			Status: status,
		})
	}

	return NetStats{
		Hostname:   hostname,
		Interfaces: interfaces,
	}
}

// handleSystemStats handles the GET /api/system/stats endpoint.
func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := gatherSystemStats()
	if err != nil {
		s.jsonError(w, fmt.Sprintf("failed to gather stats: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
