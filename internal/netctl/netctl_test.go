package netctl

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeTailscale swaps the injected lookup/run functions for the duration of a
// test and records the args of every tailscale invocation.
func fakeTailscale(t *testing.T, lookErr error, output []byte, runErr error) *[][]string {
	t.Helper()

	origLook, origRun := lookTailscale, runTailscale
	t.Cleanup(func() {
		lookTailscale, runTailscale = origLook, origRun
	})

	var calls [][]string
	lookTailscale = func() error { return lookErr }
	runTailscale = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		return output, runErr
	}
	return &calls
}

// fakePing swaps the injected ping runner for the duration of a test and
// records the args of every ping invocation.
func fakePing(t *testing.T, output []byte, runErr error) *[][]string {
	t.Helper()

	origRun := runPing
	t.Cleanup(func() {
		runPing = origRun
	})

	var calls [][]string
	runPing = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		return output, runErr
	}
	return &calls
}

func TestTailscaleStatus(t *testing.T) {
	tests := []struct {
		name     string
		lookErr  error
		output   string
		runErr   error
		wantArgs []string
		want     *TailscaleState
		wantErr  string
	}{
		{
			name: "parses self and peers sorted by host name",
			output: `{
				"BackendState": "Running",
				"Self": {"HostName": "axios-box", "TailscaleIPs": ["100.64.0.1", "fd7a::1"], "Online": true, "OS": "linux"},
				"Peer": {
					"nodekey:bbb": {"HostName": "phone", "TailscaleIPs": ["100.64.0.3"], "Online": false, "OS": "iOS"},
					"nodekey:aaa": {"HostName": "laptop", "TailscaleIPs": ["100.64.0.2"], "Online": true, "OS": "macOS"}
				}
			}`,
			wantArgs: []string{"status", "--json"},
			want: &TailscaleState{
				Self: TailscaleSelf{HostName: "axios-box", TailscaleIPs: []string{"100.64.0.1", "fd7a::1"}, Online: true},
				Peers: []TailscalePeer{
					{HostName: "laptop", TailscaleIPs: []string{"100.64.0.2"}, Online: true, OS: "macOS"},
					{HostName: "phone", TailscaleIPs: []string{"100.64.0.3"}, Online: false, OS: "iOS"},
				},
			},
		},
		{
			name:     "no peers yields empty slice",
			output:   `{"Self": {"HostName": "axios-box", "TailscaleIPs": ["100.64.0.1"], "Online": true}}`,
			wantArgs: []string{"status", "--json"},
			want: &TailscaleState{
				Self:  TailscaleSelf{HostName: "axios-box", TailscaleIPs: []string{"100.64.0.1"}, Online: true},
				Peers: []TailscalePeer{},
			},
		},
		{
			name:    "missing binary",
			lookErr: errors.New("executable file not found in $PATH"),
			wantErr: "tailscale CLI not found in PATH",
		},
		{
			name:    "command failure",
			output:  "Tailscale is stopped.",
			runErr:  errors.New("exit status 1"),
			wantErr: "tailscale status failed: Tailscale is stopped.",
		},
		{
			name:    "invalid JSON",
			output:  "not json",
			wantErr: "failed to parse tailscale status output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeTailscale(t, tt.lookErr, []byte(tt.output), tt.runErr)

			got, err := TailscaleStatus()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("TailscaleStatus() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("TailscaleStatus() error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("TailscaleStatus() error = %v", err)
			}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], tt.wantArgs) {
				t.Errorf("tailscale args = %v, want [%v]", *calls, tt.wantArgs)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TailscaleStatus() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestTailscaleUpDown(t *testing.T) {
	tests := []struct {
		name     string
		call     func() (string, error)
		wantArgs []string
	}{
		{"up", TailscaleUp, []string{"up"}},
		{"down", TailscaleDown, []string{"down"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeTailscale(t, nil, []byte("done\n"), nil)

			out, err := tt.call()
			if err != nil {
				t.Fatalf("Tailscale%s() error = %v", tt.name, err)
			}
			if out != "done" {
				t.Errorf("output = %q, want %q", out, "done")
			}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], tt.wantArgs) {
				t.Errorf("tailscale args = %v, want [%v]", *calls, tt.wantArgs)
			}
		})

		t.Run(tt.name+" missing binary", func(t *testing.T) {
			calls := fakeTailscale(t, errors.New("not found"), nil, nil)

			if _, err := tt.call(); err == nil || !strings.Contains(err.Error(), "tailscale CLI not found in PATH") {
				t.Fatalf("error = %v, want missing-binary error", err)
			}
			if len(*calls) != 0 {
				t.Errorf("tailscale was executed despite missing binary: %v", *calls)
			}
		})
	}
}

func TestPingHost(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		count    int
		output   string
		runErr   error
		wantArgs []string
		want     string
		wantErr  string
	}{
		{
			name:     "builds args from host and count",
			host:     "example.com",
			count:    3,
			output:   "3 packets transmitted, 3 received\n",
			wantArgs: []string{"-c", "3", "example.com"},
			want:     "3 packets transmitted, 3 received",
		},
		{
			name:     "ipv4 literal",
			host:     "192.168.1.1",
			count:    2,
			output:   "ok",
			wantArgs: []string{"-c", "2", "192.168.1.1"},
			want:     "ok",
		},
		{
			name:     "ipv6 literal",
			host:     "::1",
			count:    1,
			output:   "ok",
			wantArgs: []string{"-c", "1", "::1"},
			want:     "ok",
		},
		{
			name:     "count below range clamps to 1",
			host:     "example.com",
			count:    0,
			output:   "ok",
			wantArgs: []string{"-c", "1", "example.com"},
			want:     "ok",
		},
		{
			name:     "negative count clamps to 1",
			host:     "example.com",
			count:    -7,
			output:   "ok",
			wantArgs: []string{"-c", "1", "example.com"},
			want:     "ok",
		},
		{
			name:     "count above range clamps to 5",
			host:     "example.com",
			count:    99,
			output:   "ok",
			wantArgs: []string{"-c", "5", "example.com"},
			want:     "ok",
		},
		{
			name:    "rejects command injection",
			host:    "example.com; rm -rf /",
			count:   3,
			wantErr: "invalid host",
		},
		{
			name:    "rejects spaces",
			host:    "example .com",
			count:   3,
			wantErr: "invalid host",
		},
		{
			name:    "rejects backticks",
			host:    "`whoami`.example.com",
			count:   3,
			wantErr: "invalid host",
		},
		{
			name:    "rejects option injection",
			host:    "-f",
			count:   3,
			wantErr: "invalid host",
		},
		{
			name:    "rejects empty host",
			host:    "",
			count:   3,
			wantErr: "host must not be empty",
		},
		{
			name:    "ping failure surfaces output",
			host:    "example.com",
			count:   3,
			output:  "ping: cannot resolve example.com",
			runErr:  errors.New("exit status 68"),
			wantErr: "ping failed: ping: cannot resolve example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakePing(t, []byte(tt.output), tt.runErr)

			got, err := PingHost(tt.host, tt.count)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("PingHost() error = nil, want %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("PingHost() error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				if tt.runErr == nil && len(*calls) != 0 {
					t.Errorf("ping was executed for invalid host: %v", *calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("PingHost() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("PingHost() = %q, want %q", got, tt.want)
			}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], tt.wantArgs) {
				t.Errorf("ping args = %v, want [%v]", *calls, tt.wantArgs)
			}
		})
	}
}

func TestValidateHost(t *testing.T) {
	tests := []struct {
		host    string
		wantErr bool
	}{
		{"example.com", false},
		{"sub-domain.example.com", false},
		{"host_1", false},
		{"192.168.1.1", false},
		{"::1", false},
		{"fd7a:115c:a1e0::1", false},
		{"", true},
		{"; rm -rf /", true},
		{"host name", true},
		{"`whoami`", true},
		{"$(reboot)", true},
		{"-c", true},
		{"host|cat", true},
		{strings.Repeat("a", 254), true},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			err := ValidateHost(tt.host)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHost(%q) error = %v, wantErr %v", tt.host, err, tt.wantErr)
			}
		})
	}
}

func TestDNSLookupLocalhost(t *testing.T) {
	result, err := DNSLookup("localhost")
	if err != nil {
		t.Fatalf("DNSLookup(localhost) error = %v", err)
	}
	if result.Host != "localhost" {
		t.Errorf("Host = %q, want %q", result.Host, "localhost")
	}
	if len(result.Addresses) == 0 {
		t.Fatal("DNSLookup(localhost) returned no addresses")
	}
	for _, addr := range result.Addresses {
		if addr == "127.0.0.1" || addr == "::1" {
			return
		}
	}
	t.Errorf("Addresses = %v, want to include 127.0.0.1 or ::1", result.Addresses)
}

func TestDNSLookupEmptyHost(t *testing.T) {
	if _, err := DNSLookup(""); err == nil || !strings.Contains(err.Error(), "host must not be empty") {
		t.Fatalf("DNSLookup(\"\") error = %v, want empty-host error", err)
	}
}

func TestListInterfaces(t *testing.T) {
	ifaces, err := ListInterfaces()
	if err != nil {
		t.Fatalf("ListInterfaces() error = %v", err)
	}
	if len(ifaces) == 0 {
		t.Fatal("ListInterfaces() returned no interfaces")
	}

	for _, iface := range ifaces {
		if iface.Name == "" {
			t.Errorf("interface with empty name: %+v", iface)
		}
		if iface.IPs == nil {
			t.Errorf("interface %s has nil IPs slice, want empty slice", iface.Name)
		}
	}

	// Every machine running the tests has a loopback interface.
	for _, iface := range ifaces {
		for _, ip := range iface.IPs {
			if strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "::1") {
				return
			}
		}
	}
	t.Error("ListInterfaces() did not include a loopback interface")
}
