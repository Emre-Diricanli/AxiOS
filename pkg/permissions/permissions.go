// Package permissions implements the AxiOS tiered trust model.
//
// The version-2 schema (configs/permissions.yaml) keys entries by actual
// runtime tool names ("server__tool") with "*"/"?" wildcards. Entries may
// optionally carry an argument pattern in the "tool:pattern" form, which
// additionally matches the primary path-like argument of the call (the first
// present string value among the keys "path", "file", "target"). Evaluation
// order is prohibited -> trusted -> approval_required -> default tier.
//
// The legacy version-1 schema (a "tiers:" block with operation lists such as
// "fs:read") is still loaded: each operation maps onto the nearest v2
// semantics — an exact tool-name entry — and the returned Config carries a
// deprecation note.
//
// The package is pure: Check performs no I/O and no logging; the only I/O is
// reading the file in LoadConfig.
package permissions

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Tier is a trust tier for tool execution.
type Tier string

const (
	Trusted          Tier = "trusted"
	ApprovalRequired Tier = "approval_required"
	Prohibited       Tier = "prohibited"
)

// currentVersion is the newest supported schema version.
const currentVersion = 2

// rule is a single parsed permission entry. Plain entries ("axios-fs__read_file",
// "opencode__*") match the tool name only. Entries in the "tool:pattern" form
// also require the call's primary path-like argument to match argPattern.
type rule struct {
	toolPattern string
	argPattern  string
	hasArg      bool
}

// Config holds the permission tiers loaded from permissions.yaml.
type Config struct {
	// Version is the schema version of the loaded file: 2 for the current
	// schema, 1 for a legacy "tiers:" file.
	Version int
	// DefaultTier applies to tools that match no entry.
	DefaultTier Tier
	// Deprecation is a human-readable note set when a legacy version-1 file
	// was loaded; empty otherwise.
	Deprecation string

	prohibited []rule
	trusted    []rule
	approval   []rule

	// home is the user's home directory, resolved once at load time so that
	// Check stays free of environment lookups. Empty disables ~ expansion.
	home string
}

// rawConfig can decode both the version-2 schema and the legacy version-1
// "tiers:" schema.
type rawConfig struct {
	Version          int          `yaml:"version"`
	DefaultTier      string       `yaml:"default_tier"`
	Prohibited       []string     `yaml:"prohibited"`
	Trusted          []string     `yaml:"trusted"`
	ApprovalRequired []string     `yaml:"approval_required"`
	Tiers            *legacyTiers `yaml:"tiers"`
}

type legacyTiers struct {
	Trusted          []string `yaml:"trusted"`
	ApprovalRequired []string `yaml:"approval_required"`
	Prohibited       []string `yaml:"prohibited"`
}

// LoadConfig reads and parses a permissions.yaml file. Both the version-2
// schema and the legacy version-1 schema are accepted; legacy files set
// Config.Deprecation.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read permissions config: %w", err)
	}
	return ParseConfig(data)
}

// ParseConfig parses permissions.yaml content that has already been read
// (or embedded). Both schemas are accepted, exactly as in LoadConfig.
func ParseConfig(data []byte) (*Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse permissions config: %w", err)
	}

	home, _ := os.UserHomeDir()
	return buildConfig(&raw, home)
}

// DefaultYAML is the built-in fallback policy, identical to the shipped
// configs/permissions.yaml. It is used when no permissions file can be
// found on disk so that enforcement is never silently disabled.
const DefaultYAML = `version: 2
default_tier: approval_required
prohibited:
  - "axios-fs__write_file:/etc/axios/*"
  - "axios-fs__read_file:~/.axios/providers.json"
  - "axios-fs__read_file:~/.axios/master.key"
trusted:
  - "axios-fs__read_file"
  - "axios-fs__list_directory"
  - "axios-fs__search_files"
  - "axios-fs__file_info"
  - "axios-system__system_info"
  - "axios-system__disk_usage"
  - "axios-system__process_list"
  - "axios-system__service_status"
approval_required:
  - "axios-fs__write_file"
  - "axios-fs__delete_file"
  - "axios-system__run_command"
  - "axios-system__reboot"
  - "opencode__*"
`

// Default returns the built-in policy parsed from DefaultYAML.
func Default() *Config {
	cfg, err := ParseConfig([]byte(DefaultYAML))
	if err != nil {
		// DefaultYAML is a compile-time constant covered by tests; parsing
		// it can only fail if the constant itself is broken.
		panic(fmt.Sprintf("permissions: built-in default config invalid: %v", err))
	}
	return cfg
}

// buildConfig converts a decoded rawConfig into a Config, validating it.
func buildConfig(raw *rawConfig, home string) (*Config, error) {
	if raw.Version > currentVersion {
		return nil, fmt.Errorf("unsupported permissions config version %d (newest supported: %d)", raw.Version, currentVersion)
	}

	defaultTier, err := parseDefaultTier(raw.DefaultTier)
	if err != nil {
		return nil, err
	}

	cfg := &Config{DefaultTier: defaultTier, home: home}

	if raw.Tiers != nil {
		// Legacy version-1 schema: a "tiers:" block with operation lists.
		if raw.Version >= currentVersion {
			return nil, fmt.Errorf("permissions config declares version %d but contains a legacy 'tiers' block", raw.Version)
		}
		if len(raw.Prohibited)+len(raw.Trusted)+len(raw.ApprovalRequired) > 0 {
			return nil, fmt.Errorf("permissions config mixes legacy 'tiers' block with top-level tier lists")
		}
		cfg.Version = 1
		cfg.Deprecation = "permissions config uses the legacy version-1 schema ('tiers' with operation lists); " +
			"migrate to version 2 (top-level prohibited/trusted/approval_required lists keyed by runtime tool names)"
		// Nearest v2 semantics: each legacy operation becomes an exact
		// tool-name entry (no arg-pattern splitting — legacy names use ':'
		// as a category separator, e.g. "fs:read").
		if cfg.prohibited, err = parseRules(raw.Tiers.Prohibited, false); err != nil {
			return nil, err
		}
		if cfg.trusted, err = parseRules(raw.Tiers.Trusted, false); err != nil {
			return nil, err
		}
		if cfg.approval, err = parseRules(raw.Tiers.ApprovalRequired, false); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	cfg.Version = currentVersion
	if cfg.prohibited, err = parseRules(raw.Prohibited, true); err != nil {
		return nil, err
	}
	if cfg.trusted, err = parseRules(raw.Trusted, true); err != nil {
		return nil, err
	}
	if cfg.approval, err = parseRules(raw.ApprovalRequired, true); err != nil {
		return nil, err
	}
	return cfg, nil
}

// parseDefaultTier validates default_tier; empty means approval_required.
func parseDefaultTier(s string) (Tier, error) {
	switch Tier(s) {
	case "":
		return ApprovalRequired, nil
	case Trusted, ApprovalRequired, Prohibited:
		return Tier(s), nil
	default:
		return "", fmt.Errorf("invalid default_tier %q (want trusted, approval_required, or prohibited)", s)
	}
}

// parseRules converts raw entries into rules. When allowArgPattern is true,
// entries may use the "tool:pattern" form; otherwise the whole entry is a
// tool-name pattern.
func parseRules(entries []string, allowArgPattern bool) ([]rule, error) {
	rules := make([]rule, 0, len(entries))
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			return nil, fmt.Errorf("empty permission entry")
		}
		r := rule{toolPattern: e}
		if allowArgPattern {
			if i := strings.Index(e, ":"); i >= 0 {
				tool, pat := e[:i], e[i+1:]
				if tool == "" || pat == "" {
					return nil, fmt.Errorf("invalid permission entry %q: want \"tool\" or \"tool:pattern\"", e)
				}
				r = rule{toolPattern: tool, argPattern: pat, hasArg: true}
			}
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// Check returns the trust tier for a tool call. Evaluation order is
// prohibited -> trusted -> approval_required; tools matching no entry get
// DefaultTier. Entries with an argument pattern only apply when the call has
// a primary path-like argument (first present string value among the keys
// "path", "file", "target"); a leading "~" in either the pattern or the
// argument is expanded to the user's home directory before matching.
func (c *Config) Check(toolName string, args map[string]any) Tier {
	arg, hasArg := primaryArg(args)
	if c.matches(c.prohibited, toolName, arg, hasArg) {
		return Prohibited
	}
	if c.matches(c.trusted, toolName, arg, hasArg) {
		return Trusted
	}
	if c.matches(c.approval, toolName, arg, hasArg) {
		return ApprovalRequired
	}
	return c.DefaultTier
}

// primaryArg returns the first present string value among the keys
// "path", "file", "target". Non-string values are skipped.
func primaryArg(args map[string]any) (string, bool) {
	for _, key := range []string{"path", "file", "target"} {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok {
				return s, true
			}
		}
	}
	return "", false
}

func (c *Config) matches(rules []rule, toolName, arg string, hasArg bool) bool {
	for _, r := range rules {
		if !wildcardMatch(r.toolPattern, toolName) {
			continue
		}
		if !r.hasArg {
			return true
		}
		if !hasArg {
			continue
		}
		if wildcardMatch(c.expandHome(r.argPattern), c.expandHome(arg)) {
			return true
		}
	}
	return false
}

// expandHome rewrites a leading "~" or "~/..." to the user's home directory
// so patterns written either way match arguments written either way.
// The "~user" form is not supported and is left untouched.
func (c *Config) expandHome(s string) string {
	if c.home == "" || s == "" || s[0] != '~' {
		return s
	}
	if s == "~" {
		return c.home
	}
	if strings.HasPrefix(s, "~/") {
		return c.home + s[1:]
	}
	return s
}

// wildcardMatch reports whether s matches pattern, where '*' matches any
// sequence of characters (including none, and across '/' separators) and '?'
// matches exactly one character. filepath.Match is not used because its '*'
// stops at path separators, which is wrong for multi-segment patterns like
// "axios-fs__write_file:/etc/axios/*".
func wildcardMatch(pattern, s string) bool {
	p := []rune(pattern)
	t := []rune(s)
	px, tx := 0, 0
	starPx, starTx := -1, 0
	for tx < len(t) {
		switch {
		case px < len(p) && (p[px] == '?' || p[px] == t[tx]):
			px++
			tx++
		case px < len(p) && p[px] == '*':
			starPx, starTx = px, tx
			px++
		case starPx >= 0:
			// Backtrack: let the last '*' consume one more rune.
			starTx++
			px = starPx + 1
			tx = starTx
		default:
			return false
		}
	}
	for px < len(p) && p[px] == '*' {
		px++
	}
	return px == len(p)
}
