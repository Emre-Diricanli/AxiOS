package obsidianctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// ErrNotConfigured is returned when no vault has been configured from any
// source (override flag, state file, or config seed). It is a normal state —
// callers translate it into a friendly "set the vault first" message.
var ErrNotConfigured = errors.New("no vault configured")

// stateFileName is the runtime vault-selection state inside $AXIOS_DATA_DIR.
const stateFileName = "obsidian.json"

// vaultState is the on-disk shape of obsidian.json.
type vaultState struct {
	VaultPath string `json:"vault_path"`
}

// Manager resolves which vault directory is active. Precedence: explicit
// override (--vault flag / tests) > the obsidian.json state file (written by
// the daemon on PUT /api/obsidian/vault) > the config seed. The state file is
// re-read on every call — a cheap stat+read — so a vault switch made in the
// web UI propagates to the axios-obsidian MCP server process without a
// restart.
type Manager struct {
	mu        sync.Mutex
	statePath string
	override  string
	seed      string
}

// NewManager creates a Manager persisting its state under dataDir
// ($AXIOS_DATA_DIR). seed is the config fallback used until a vault has been
// set explicitly; empty means unconfigured.
func NewManager(dataDir, seed string) *Manager {
	return &Manager{statePath: filepath.Join(dataDir, stateFileName), seed: seed}
}

// SetOverride pins the vault path, bypassing the state file and the seed
// (the MCP server's --vault flag). Empty clears the override.
func (m *Manager) SetOverride(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.override = path
}

// Path resolves the active vault directory without opening it:
// override > state file > seed. It returns ErrNotConfigured when no source
// names a vault, and an error when the state file exists but is unreadable.
func (m *Manager) Path() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.override != "" {
		return m.override, nil
	}

	data, err := os.ReadFile(m.statePath)
	switch {
	case err == nil:
		var st vaultState
		if jerr := json.Unmarshal(data, &st); jerr != nil {
			return "", fmt.Errorf("parse vault state %s: %w", m.statePath, jerr)
		}
		if st.VaultPath != "" {
			return st.VaultPath, nil
		}
	case !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("read vault state %s: %w", m.statePath, err)
	}

	if m.seed != "" {
		return m.seed, nil
	}
	return "", ErrNotConfigured
}

// Vault opens the currently configured vault. An unconfigured manager
// returns ErrNotConfigured — never a panic.
func (m *Manager) Vault() (*Vault, error) {
	path, err := m.Path()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

// SetVault validates path (absolute, existing directory) and persists it to
// the state file, from where every subsequent Vault() call — in this process
// and in the axios-obsidian MCP server — picks it up.
func (m *Manager) SetVault(path string) (*Vault, error) {
	vault, err := Open(path)
	if err != nil {
		return nil, err
	}

	data, err := json.MarshalIndent(vaultState{VaultPath: vault.Root()}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode vault state: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir for vault state: %w", err)
	}
	if err := os.WriteFile(m.statePath, append(data, '\n'), 0o644); err != nil {
		return nil, fmt.Errorf("write vault state %s: %w", m.statePath, err)
	}
	return vault, nil
}

// SeedState persists the config seed into the state file when no state file
// exists yet, making the seed visible to other processes (the MCP server).
// Existing state always wins and is never overwritten; an empty seed is a
// no-op.
func (m *Manager) SeedState() error {
	m.mu.Lock()
	seed, statePath := m.seed, m.statePath
	m.mu.Unlock()

	if seed == "" {
		return nil
	}
	if _, err := os.Stat(statePath); err == nil {
		return nil // state file already written — it wins over the seed
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat vault state %s: %w", statePath, err)
	}
	if _, err := m.SetVault(seed); err != nil {
		return fmt.Errorf("seed vault state from config: %w", err)
	}
	return nil
}
