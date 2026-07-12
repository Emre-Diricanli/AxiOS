// Package netctl wraps network inspection (interfaces, DNS, ping) and the
// tailscale CLI for VPN status and connectivity. It backs the axios-network
// MCP server so network logic lives in one place.
package netctl

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Command execution is injected behind package-level function variables so
// tests can fake ping/tailscale CLI output without touching the network.
var (
	// lookTailscale reports whether the tailscale CLI is on PATH.
	lookTailscale = func() error {
		_, err := exec.LookPath("tailscale")
		return err
	}
	// runTailscale executes `tailscale` with args and returns combined stdout+stderr.
	runTailscale = func(args ...string) ([]byte, error) {
		return exec.Command("tailscale", args...).CombinedOutput()
	}
	// runPing executes `ping` with args and returns combined stdout+stderr.
	runPing = func(args ...string) ([]byte, error) {
		return exec.Command("ping", args...).CombinedOutput()
	}
)

const (
	// dnsTimeout bounds DNS lookups so a dead resolver cannot hang a tool call.
	dnsTimeout = 5 * time.Second
	// minPingCount and maxPingCount clamp the ping packet count.
	minPingCount = 1
	maxPingCount = 5
	// maxHostLen is the RFC 1035 limit on a fully qualified domain name.
	maxHostLen = 253
)

// hostPattern matches a conservative subset of hostnames and IP literals:
// letters, digits, dots, hyphens, underscores, and colons (IPv6). The first
// character must be alphanumeric or a colon, which blocks option injection
// (leading "-") as well as shell metacharacters, spaces, and backticks.
var hostPattern = regexp.MustCompile(`^[A-Za-z0-9:][A-Za-z0-9._:-]*$`)

// --- Network data types ---

// NetworkInterface describes one interface from net.Interfaces().
type NetworkInterface struct {
	Name         string   `json:"name"`
	Up           bool     `json:"up"`
	MTU          int      `json:"mtu"`
	HardwareAddr string   `json:"hardware_addr,omitempty"`
	IPs          []string `json:"ips"`
}

// DNSResult holds the outcome of a DNS lookup.
type DNSResult struct {
	Host      string   `json:"host"`
	Addresses []string `json:"addresses"`
	CNAME     string   `json:"cname,omitempty"`
}

// TailscaleSelf describes this machine in the tailnet.
type TailscaleSelf struct {
	HostName     string   `json:"host_name"`
	TailscaleIPs []string `json:"tailscale_ips"`
	Online       bool     `json:"online"`
}

// TailscalePeer describes another machine in the tailnet.
type TailscalePeer struct {
	HostName     string   `json:"host_name"`
	TailscaleIPs []string `json:"tailscale_ips"`
	Online       bool     `json:"online"`
	OS           string   `json:"os,omitempty"`
}

// TailscaleState is the parsed output of `tailscale status --json`.
type TailscaleState struct {
	Self  TailscaleSelf   `json:"self"`
	Peers []TailscalePeer `json:"peers"`
}

// rawTailscalePeer mirrors the field names tailscale emits in its JSON.
type rawTailscalePeer struct {
	HostName     string   `json:"HostName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online       bool     `json:"Online"`
	OS           string   `json:"OS"`
}

// rawTailscaleStatus mirrors the top-level `tailscale status --json` shape.
type rawTailscaleStatus struct {
	Self rawTailscalePeer            `json:"Self"`
	Peer map[string]rawTailscalePeer `json:"Peer"`
}

// --- Network operations ---

// ValidateHost checks that host is a plausible hostname or IP literal and
// rejects anything containing shell metacharacters, spaces, or option-like
// prefixes before it can reach an exec'd command line.
func ValidateHost(host string) error {
	if host == "" {
		return fmt.Errorf("host must not be empty")
	}
	if len(host) > maxHostLen {
		return fmt.Errorf("host exceeds %d characters", maxHostLen)
	}
	if !hostPattern.MatchString(host) {
		return fmt.Errorf("invalid host %q: only letters, digits, '.', '-', '_' and ':' are allowed", host)
	}
	return nil
}

// ListInterfaces enumerates network interfaces via the Go standard library
// (no external commands) and returns name, state, MTU, hardware address, and
// assigned IPs for each.
func ListInterfaces() ([]NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}

	out := make([]NetworkInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		ni := NetworkInterface{
			Name:         iface.Name,
			Up:           iface.Flags&net.FlagUp != 0,
			MTU:          iface.MTU,
			HardwareAddr: iface.HardwareAddr.String(),
			IPs:          []string{},
		}
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				ni.IPs = append(ni.IPs, addr.String())
			}
		}
		out = append(out, ni)
	}
	return out, nil
}

// DNSLookup resolves host to its addresses via net.LookupHost semantics and
// additionally reports the CNAME when one exists (best effort). Lookups are
// bounded by a 5-second context.
func DNSLookup(host string) (*DNSResult, error) {
	if host == "" {
		return nil, fmt.Errorf("host must not be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), dnsTimeout)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("dns lookup failed for %s: %w", host, err)
	}

	result := &DNSResult{Host: host, Addresses: addrs}

	// CNAME resolution is best effort: many hosts have none, and a failure
	// here must not mask a successful address lookup.
	if cname, err := net.DefaultResolver.LookupCNAME(ctx, host); err == nil {
		cname = strings.TrimSuffix(cname, ".")
		if cname != "" && cname != host {
			result.CNAME = cname
		}
	}
	return result, nil
}

// PingHost runs `ping -c <count> <host>` and returns the combined output.
// The host is validated against a strict hostname/IP pattern and count is
// clamped to [1, 5].
func PingHost(host string, count int) (string, error) {
	if err := ValidateHost(host); err != nil {
		return "", err
	}
	if count < minPingCount {
		count = minPingCount
	}
	if count > maxPingCount {
		count = maxPingCount
	}

	out, err := runPing("-c", strconv.Itoa(count), host)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return "", fmt.Errorf("ping failed: %w", err)
		}
		return "", fmt.Errorf("ping failed: %s", msg)
	}
	return strings.TrimSpace(string(out)), nil
}

// TailscaleAvailable checks whether the tailscale CLI is installed.
func TailscaleAvailable() error {
	if err := lookTailscale(); err != nil {
		return fmt.Errorf("tailscale CLI not found in PATH: %w", err)
	}
	return nil
}

// TailscaleStatus runs `tailscale status --json` and parses the self node
// and peers, sorted by host name for deterministic output.
func TailscaleStatus() (*TailscaleState, error) {
	if err := TailscaleAvailable(); err != nil {
		return nil, err
	}

	out, err := runTailscale("status", "--json")
	if err != nil {
		return nil, fmt.Errorf("tailscale status failed: %s", strings.TrimSpace(string(out)))
	}

	var raw rawTailscaleStatus
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse tailscale status output: %w", err)
	}

	state := &TailscaleState{
		Self: TailscaleSelf{
			HostName:     raw.Self.HostName,
			TailscaleIPs: raw.Self.TailscaleIPs,
			Online:       raw.Self.Online,
		},
		Peers: make([]TailscalePeer, 0, len(raw.Peer)),
	}
	for _, p := range raw.Peer {
		state.Peers = append(state.Peers, TailscalePeer{
			HostName:     p.HostName,
			TailscaleIPs: p.TailscaleIPs,
			Online:       p.Online,
			OS:           p.OS,
		})
	}
	sort.Slice(state.Peers, func(i, j int) bool {
		return state.Peers[i].HostName < state.Peers[j].HostName
	})
	return state, nil
}

// TailscaleUp runs `tailscale up` and returns the combined output.
func TailscaleUp() (string, error) {
	if err := TailscaleAvailable(); err != nil {
		return "", err
	}

	out, err := runTailscale("up")
	if err != nil {
		return "", fmt.Errorf("tailscale up failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// TailscaleDown runs `tailscale down` and returns the combined output.
func TailscaleDown() (string, error) {
	if err := TailscaleAvailable(); err != nil {
		return "", err
	}

	out, err := runTailscale("down")
	if err != nil {
		return "", fmt.Errorf("tailscale down failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
