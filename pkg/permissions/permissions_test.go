package permissions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes yaml content to a temp file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "permissions.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// loadConfig loads yaml content and pins the home directory for hermetic
// ~ expansion tests.
func loadConfig(t *testing.T, content, home string) *Config {
	t.Helper()
	cfg, err := LoadConfig(writeConfig(t, content))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	cfg.home = home
	return cfg
}

const v2Config = `
version: 2
default_tier: approval_required
prohibited:
  - "axios-fs__write_file:/etc/axios/*"
  - "axios-fs__read_file:~/.axios/providers.json"
  - "axios-fs__read_file:~/.axios/master.key"
  - "axios-system__disable_auth"
trusted:
  - "axios-fs__read_file"
  - "axios-fs__list_directory"
  - "axios-system__system_info"
  - "axios-system__disable_auth"
  - "axios-gpu__query_?"
approval_required:
  - "axios-fs__write_file"
  - "axios-fs__delete_file"
  - "axios-system__run_command"
  - "opencode__*"
`

func TestCheckTierEvaluation(t *testing.T) {
	cfg := loadConfig(t, v2Config, "/home/testuser")

	tests := []struct {
		name string
		tool string
		args map[string]any
		want Tier
	}{
		{"trusted exact", "axios-fs__read_file", nil, Trusted},
		{"trusted exact list_directory", "axios-fs__list_directory", nil, Trusted},
		{"approval exact", "axios-system__run_command", map[string]any{"command": "ls"}, ApprovalRequired},
		{"prohibited beats trusted", "axios-system__disable_auth", nil, Prohibited},
		{"unknown tool gets default", "axios-docker__list_containers", nil, ApprovalRequired},
		{"wildcard star matches suffix", "opencode__delegate_task", nil, ApprovalRequired},
		{"wildcard star matches empty suffix", "opencode__", nil, ApprovalRequired},
		{"wildcard star does not match different prefix", "opencoder__task", nil, ApprovalRequired}, // default, not the opencode__* entry
		{"question mark matches one char", "axios-gpu__query_a", nil, Trusted},
		{"question mark rejects zero chars", "axios-gpu__query_", nil, ApprovalRequired},
		{"question mark rejects two chars", "axios-gpu__query_ab", nil, ApprovalRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.Check(tt.tool, tt.args); got != tt.want {
				t.Errorf("Check(%q, %v) = %q, want %q", tt.tool, tt.args, got, tt.want)
			}
		})
	}
}

func TestDefaultTierOverride(t *testing.T) {
	cfg := loadConfig(t, `
version: 2
default_tier: trusted
prohibited:
  - "axios-system__reboot"
`, "")

	if got := cfg.Check("anything__goes", nil); got != Trusted {
		t.Errorf("unknown tool with default_tier trusted = %q, want %q", got, Trusted)
	}
	if got := cfg.Check("axios-system__reboot", nil); got != Prohibited {
		t.Errorf("prohibited tool = %q, want %q", got, Prohibited)
	}
}

func TestCheckArgPatternForm(t *testing.T) {
	const home = "/home/testuser"
	cfg := loadConfig(t, `
version: 2
prohibited:
  - "axios-fs__write_file:/etc/axios/*"
  - "axios-fs__read_file:~/.axios/providers.json"
  - "axios-fs__read_file:/home/testuser/.axios/master.key"
trusted:
  - "axios-fs__read_file"
approval_required:
  - "axios-fs__write_file"
`, home)

	tests := []struct {
		name string
		tool string
		args map[string]any
		want Tier
	}{
		{"path inside prohibited pattern", "axios-fs__write_file",
			map[string]any{"path": "/etc/axios/permissions.yaml"}, Prohibited},
		{"multi-segment star crosses separators", "axios-fs__write_file",
			map[string]any{"path": "/etc/axios/sub/dir/file.yaml"}, Prohibited},
		{"path outside pattern falls through to approval", "axios-fs__write_file",
			map[string]any{"path": "/home/testuser/notes.txt"}, ApprovalRequired},
		{"arg rule skipped without path-like arg", "axios-fs__write_file",
			map[string]any{"content": "x"}, ApprovalRequired},
		{"arg rule skipped with nil args", "axios-fs__write_file", nil, ApprovalRequired},
		{"tilde pattern matches absolute arg", "axios-fs__read_file",
			map[string]any{"path": home + "/.axios/providers.json"}, Prohibited},
		{"tilde pattern matches tilde arg", "axios-fs__read_file",
			map[string]any{"path": "~/.axios/providers.json"}, Prohibited},
		{"absolute pattern matches tilde arg", "axios-fs__read_file",
			map[string]any{"path": "~/.axios/master.key"}, Prohibited},
		{"non-matching path stays trusted", "axios-fs__read_file",
			map[string]any{"path": home + "/documents/todo.md"}, Trusted},
		{"file key is honored", "axios-fs__write_file",
			map[string]any{"file": "/etc/axios/axiosd.yaml"}, Prohibited},
		{"target key is honored", "axios-fs__write_file",
			map[string]any{"target": "/etc/axios/axiosd.yaml"}, Prohibited},
		{"path takes precedence over file", "axios-fs__write_file",
			map[string]any{"path": "/tmp/safe", "file": "/etc/axios/axiosd.yaml"}, ApprovalRequired},
		{"non-string path falls through to file key", "axios-fs__write_file",
			map[string]any{"path": 123, "file": "/etc/axios/axiosd.yaml"}, Prohibited},
		{"non-string values everywhere means no arg", "axios-fs__write_file",
			map[string]any{"path": 1, "file": true, "target": []string{"/etc/axios/x"}}, ApprovalRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.Check(tt.tool, tt.args); got != tt.want {
				t.Errorf("Check(%q, %v) = %q, want %q", tt.tool, tt.args, got, tt.want)
			}
		})
	}
}

func TestCheckArgPatternNoHomeDir(t *testing.T) {
	// With no resolvable home, ~ expansion is disabled: tilde patterns only
	// match literal tilde args.
	cfg := loadConfig(t, `
version: 2
prohibited:
  - "axios-fs__read_file:~/.axios/master.key"
`, "")

	if got := cfg.Check("axios-fs__read_file", map[string]any{"path": "~/.axios/master.key"}); got != Prohibited {
		t.Errorf("literal tilde arg = %q, want %q", got, Prohibited)
	}
	if got := cfg.Check("axios-fs__read_file", map[string]any{"path": "/home/x/.axios/master.key"}); got != ApprovalRequired {
		t.Errorf("absolute arg without home = %q, want %q", got, ApprovalRequired)
	}
}

func TestLoadLegacyV1(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `
tiers:
  trusted:
    - fs:read
    - docker:list
  approval_required:
    - fs:delete
    - system:reboot
  prohibited:
    - system:disable_auth
    - system:export_credentials
`))
	if err != nil {
		t.Fatalf("LoadConfig legacy: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if cfg.Deprecation == "" {
		t.Error("Deprecation note is empty for legacy config")
	}
	if cfg.DefaultTier != ApprovalRequired {
		t.Errorf("DefaultTier = %q, want %q", cfg.DefaultTier, ApprovalRequired)
	}

	tests := []struct {
		op   string
		want Tier
	}{
		{"fs:read", Trusted},
		{"docker:list", Trusted},
		{"fs:delete", ApprovalRequired},
		{"system:disable_auth", Prohibited},
		// Legacy entries are exact tool names: the colon is not an arg separator.
		{"fs:read_other", ApprovalRequired},
		{"unknown:op", ApprovalRequired},
	}
	for _, tt := range tests {
		if got := cfg.Check(tt.op, nil); got != tt.want {
			t.Errorf("Check(%q) = %q, want %q", tt.op, got, tt.want)
		}
	}
}

func TestLoadConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"malformed yaml", "version: [2\n  broken", "parse permissions config"},
		{"wrong type for tier list", "version: 2\ntrusted: notalist\n", "parse permissions config"},
		{"unsupported version", "version: 3\ntrusted:\n  - \"a__b\"\n", "unsupported permissions config version 3"},
		{"invalid default tier", "version: 2\ndefault_tier: bogus\n", `invalid default_tier "bogus"`},
		{"v2 with legacy tiers block", "version: 2\ntiers:\n  trusted:\n    - fs:read\n", "legacy 'tiers' block"},
		{"legacy mixed with v2 lists", "tiers:\n  trusted:\n    - fs:read\ntrusted:\n  - \"a__b\"\n", "mixes legacy 'tiers' block"},
		{"empty entry", "version: 2\ntrusted:\n  - \"\"\n", "empty permission entry"},
		{"entry with empty tool part", "version: 2\nprohibited:\n  - \":/etc/axios/*\"\n", "invalid permission entry"},
		{"entry with empty pattern part", "version: 2\nprohibited:\n  - \"axios-fs__write_file:\"\n", "invalid permission entry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, tt.content))
			if err == nil {
				t.Fatalf("LoadConfig succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("LoadConfig succeeded for a missing file")
	}
	if !strings.Contains(err.Error(), "read permissions config") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "read permissions config")
	}
}

// TestShippedConfig validates the repository's configs/permissions.yaml.
func TestShippedConfig(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "configs", "permissions.yaml"))
	if err != nil {
		t.Fatalf("LoadConfig shipped config: %v", err)
	}
	cfg.home = "/home/testuser"

	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2", cfg.Version)
	}
	if cfg.Deprecation != "" {
		t.Errorf("Deprecation = %q, want empty for v2 config", cfg.Deprecation)
	}

	tests := []struct {
		name string
		tool string
		args map[string]any
		want Tier
	}{
		{"read_file trusted", "axios-fs__read_file", map[string]any{"path": "/tmp/x"}, Trusted},
		{"system_info trusted", "axios-system__system_info", nil, Trusted},
		{"git_status trusted", "axios-git__git_status", map[string]any{"repo": "/tmp/x"}, Trusted},
		{"run_command needs approval", "axios-system__run_command", map[string]any{"command": "ls"}, ApprovalRequired},
		{"reboot needs approval", "axios-system__reboot", nil, ApprovalRequired},
		{"git_push needs approval", "axios-git__git_push", map[string]any{"repo": "/tmp/x"}, ApprovalRequired},
		{"opencode wildcard needs approval", "opencode__delegate_task", nil, ApprovalRequired},
		{"write to /etc/axios prohibited", "axios-fs__write_file", map[string]any{"path": "/etc/axios/axiosd.yaml"}, Prohibited},
		{"read providers.json prohibited", "axios-fs__read_file", map[string]any{"path": "~/.axios/providers.json"}, Prohibited},
		{"read master.key prohibited", "axios-fs__read_file", map[string]any{"path": "/home/testuser/.axios/master.key"}, Prohibited},
		{"unknown tool defaults to approval", "axios-docker__prune", nil, ApprovalRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.Check(tt.tool, tt.args); got != tt.want {
				t.Errorf("Check(%q, %v) = %q, want %q", tt.tool, tt.args, got, tt.want)
			}
		})
	}
}

func TestWildcardMatch(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"", "", true},
		{"", "a", false},
		{"*", "", true},
		{"*", "anything/at/all", true},
		{"abc", "abc", true},
		{"abc", "abd", false},
		{"abc", "ab", false},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"a?c", "abbc", false},
		{"?", "a", true},
		{"?", "", false},
		{"opencode__*", "opencode__delegate_task", true},
		{"opencode__*", "opencode__", true},
		{"opencode__*", "opencode_x", false},
		// '*' must cross path separators (filepath.Match would fail these).
		{"/etc/axios/*", "/etc/axios/a/b/c.yaml", true},
		{"/etc/axios/*", "/etc/axios/x", true},
		{"/etc/axios/*", "/etc/axios/", true},
		{"/etc/axios/*", "/etc/other/x", false},
		{"*.yaml", "config.yaml", true},
		{"*.yaml", "dir/sub/config.yaml", true},
		{"*.yaml", "config.yml", false},
		{"a*b*c", "a-x-b-y-c", true},
		{"a*b*c", "abc", true},
		{"a*b*c", "acb", false},
		{"*a*", "bab", true},
		{"*a*", "bbb", false},
		{"a**b", "ab", true},
		{"a*?b", "ab", false},
		{"a*?b", "axb", true},
		// Backtracking: first '*' must give characters back.
		{"*abc", "aabc", true},
		{"*abc*", "xxabcyy", true},
		{"*aab", "aaab", true},
		// '?' matches one rune, not one byte.
		{"a?c", "aéc", true},
		{"~/.axios/*", "~/.axios/providers.json", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"|"+tt.s, func(t *testing.T) {
			if got := wildcardMatch(tt.pattern, tt.s); got != tt.want {
				t.Errorf("wildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
			}
		})
	}
}

// TestDefault validates the built-in fallback policy used when no
// permissions file is found on disk.
func TestDefault(t *testing.T) {
	cfg := Default()
	cfg.home = "/home/testuser"

	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2", cfg.Version)
	}
	if cfg.DefaultTier != ApprovalRequired {
		t.Errorf("DefaultTier = %q, want approval_required", cfg.DefaultTier)
	}
	if cfg.Deprecation != "" {
		t.Errorf("Deprecation = %q, want empty", cfg.Deprecation)
	}

	tests := []struct {
		name string
		tool string
		args map[string]any
		want Tier
	}{
		{"read_file trusted", "axios-fs__read_file", map[string]any{"path": "/tmp/x"}, Trusted},
		{"system_info trusted", "axios-system__system_info", nil, Trusted},
		{"git_status trusted", "axios-git__git_status", map[string]any{"repo": "/tmp/x"}, Trusted},
		{"run_command needs approval", "axios-system__run_command", map[string]any{"command": "ls"}, ApprovalRequired},
		{"git_push needs approval", "axios-git__git_push", map[string]any{"repo": "/tmp/x"}, ApprovalRequired},
		{"write to /etc/axios prohibited", "axios-fs__write_file", map[string]any{"path": "/etc/axios/axiosd.yaml"}, Prohibited},
		{"read master.key prohibited", "axios-fs__read_file", map[string]any{"path": "/home/testuser/.axios/master.key"}, Prohibited},
		{"opencode wildcard needs approval", "opencode__delegate_task", nil, ApprovalRequired},
		{"unknown tool defaults to approval", "axios-docker__prune", nil, ApprovalRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.Check(tt.tool, tt.args); got != tt.want {
				t.Errorf("Check(%q, %v) = %q, want %q", tt.tool, tt.args, got, tt.want)
			}
		})
	}
}
