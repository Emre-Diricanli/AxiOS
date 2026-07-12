package dockerctl

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeDocker swaps the injected lookup/run functions for the duration of a
// test and records the args of every docker invocation.
func fakeDocker(t *testing.T, lookErr error, output []byte, runErr error) *[][]string {
	t.Helper()

	origLook, origRun := lookDocker, runDocker
	t.Cleanup(func() {
		lookDocker, runDocker = origLook, origRun
	})

	var calls [][]string
	lookDocker = func() error { return lookErr }
	runDocker = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		return output, runErr
	}
	return &calls
}

func TestListContainers(t *testing.T) {
	tests := []struct {
		name     string
		all      bool
		output   string
		runErr   error
		wantArgs []string
		want     []DockerContainer
		wantErr  string
	}{
		{
			name: "parses running containers",
			all:  false,
			output: `{"ID":"abc123","Names":"web","Image":"nginx:latest","Status":"Up 2 hours","State":"running","Ports":"0.0.0.0:80->80/tcp","CreatedAt":"2026-01-01 10:00:00"}
{"ID":"def456","Names":"db","Image":"postgres:16","Status":"Up 1 hour","State":"running","Ports":"5432/tcp","CreatedAt":"2026-01-02 11:00:00"}
`,
			wantArgs: []string{"ps", "--format", "json"},
			want: []DockerContainer{
				{ID: "abc123", Name: "web", Image: "nginx:latest", Status: "Up 2 hours", State: "running", Ports: "0.0.0.0:80->80/tcp", Created: "2026-01-01 10:00:00"},
				{ID: "def456", Name: "db", Image: "postgres:16", Status: "Up 1 hour", State: "running", Ports: "5432/tcp", Created: "2026-01-02 11:00:00"},
			},
		},
		{
			name:     "all adds -a flag",
			all:      true,
			output:   `{"ID":"abc123","Names":"web","Image":"nginx:latest","Status":"Exited (0)","State":"exited","Ports":"","CreatedAt":"2026-01-01 10:00:00"}` + "\n",
			wantArgs: []string{"ps", "-a", "--format", "json"},
			want: []DockerContainer{
				{ID: "abc123", Name: "web", Image: "nginx:latest", Status: "Exited (0)", State: "exited", Created: "2026-01-01 10:00:00"},
			},
		},
		{
			name:     "empty output returns empty non-nil slice",
			output:   "",
			wantArgs: []string{"ps", "--format", "json"},
			want:     []DockerContainer{},
		},
		{
			name:     "skips blank and malformed lines",
			output:   "\nnot-json\n" + `{"ID":"abc123","Names":"web","Image":"nginx"}` + "\n",
			wantArgs: []string{"ps", "--format", "json"},
			want: []DockerContainer{
				{ID: "abc123", Name: "web", Image: "nginx"},
			},
		},
		{
			name:    "docker command failure propagates output",
			output:  "Cannot connect to the Docker daemon\n",
			runErr:  errors.New("exit status 1"),
			wantErr: "docker ps failed: Cannot connect to the Docker daemon",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeDocker(t, nil, []byte(tt.output), tt.runErr)

			got, err := ListContainers(tt.all)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ListContainers() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListContainers() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ListContainers() = %+v, want %+v", got, tt.want)
			}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], tt.wantArgs) {
				t.Errorf("docker args = %v, want [%v]", *calls, tt.wantArgs)
			}
		})
	}
}

func TestStopContainer(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		output  string
		runErr  error
		wantErr string
	}{
		{
			name:   "success",
			id:     "abc123",
			output: "abc123\n",
		},
		{
			name:    "docker error propagates output",
			id:      "missing",
			output:  "Error response from daemon: No such container: missing\n",
			runErr:  errors.New("exit status 1"),
			wantErr: "docker stop failed: Error response from daemon: No such container: missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeDocker(t, nil, []byte(tt.output), tt.runErr)

			err := StopContainer(tt.id)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("StopContainer() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("StopContainer() unexpected error: %v", err)
			}
			wantArgs := []string{"stop", tt.id}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], wantArgs) {
				t.Errorf("docker args = %v, want [%v]", *calls, wantArgs)
			}
		})
	}
}

func TestStats(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		runErr  error
		want    []DockerContainerStats
		wantErr string
	}{
		{
			name: "parses stats lines",
			output: `{"ID":"abc123","Name":"web","CPUPerc":"1.25%","MemUsage":"25MiB / 8GiB","MemPerc":"0.31%","NetIO":"1kB / 2kB","BlockIO":"0B / 0B","PIDs":"4"}
{"ID":"def456","Name":"db","CPUPerc":"0.10%","MemUsage":"120MiB / 8GiB","MemPerc":"1.50%","NetIO":"5kB / 3kB","BlockIO":"1MB / 0B","PIDs":"12"}
`,
			want: []DockerContainerStats{
				{ID: "abc123", Name: "web", CPUPerc: "1.25%", MemUsage: "25MiB / 8GiB", MemPerc: "0.31%", NetIO: "1kB / 2kB", BlockIO: "0B / 0B", PIDs: "4"},
				{ID: "def456", Name: "db", CPUPerc: "0.10%", MemUsage: "120MiB / 8GiB", MemPerc: "1.50%", NetIO: "5kB / 3kB", BlockIO: "1MB / 0B", PIDs: "12"},
			},
		},
		{
			name:   "no running containers yields empty non-nil slice",
			output: "",
			want:   []DockerContainerStats{},
		},
		{
			name:    "docker error propagates output",
			output:  "Cannot connect to the Docker daemon\n",
			runErr:  errors.New("exit status 1"),
			wantErr: "docker stats failed: Cannot connect to the Docker daemon",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeDocker(t, nil, []byte(tt.output), tt.runErr)

			got, err := Stats()

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Stats() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Stats() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Stats() = %+v, want %+v", got, tt.want)
			}
			wantArgs := []string{"stats", "--no-stream", "--format", "json"}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], wantArgs) {
				t.Errorf("docker args = %v, want [%v]", *calls, wantArgs)
			}
		})
	}
}

func TestDockerMissing(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{"Available", func() error { return Available() }},
		{"ListContainers", func() error { _, err := ListContainers(true); return err }},
		{"InspectContainer", func() error { _, err := InspectContainer("abc"); return err }},
		{"ContainerLogs", func() error { _, err := ContainerLogs("abc", 100); return err }},
		{"StartContainer", func() error { return StartContainer("abc") }},
		{"StopContainer", func() error { return StopContainer("abc") }},
		{"RestartContainer", func() error { return RestartContainer("abc") }},
		{"RemoveContainer", func() error { return RemoveContainer("abc", true) }},
		{"PullImage", func() error { _, err := PullImage("nginx"); return err }},
		{"RunContainer", func() error { _, err := RunContainer("nginx", "", nil, nil, nil, ""); return err }},
		{"ListImages", func() error { _, err := ListImages(); return err }},
		{"ComposeUp", func() error { _, err := ComposeUp("services: {}", "proj"); return err }},
		{"ComposeDown", func() error { _, err := ComposeDown("proj"); return err }},
		{"Stats", func() error { _, err := Stats(); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeDocker(t, errors.New("executable file not found in $PATH"), nil, nil)

			err := tt.call()

			if err == nil || !strings.Contains(err.Error(), "docker CLI not found in PATH") {
				t.Fatalf("%s error = %v, want docker-not-found error", tt.name, err)
			}
			if len(*calls) != 0 {
				t.Errorf("%s invoked docker despite missing CLI: %v", tt.name, *calls)
			}
		})
	}
}
