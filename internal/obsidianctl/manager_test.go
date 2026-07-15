package obsidianctl

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerUnconfigured(t *testing.T) {
	m := NewManager(t.TempDir(), "")

	if _, err := m.Path(); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Path() error = %v, want ErrNotConfigured", err)
	}
	if _, err := m.Vault(); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Vault() error = %v, want ErrNotConfigured", err)
	}
}

func TestManagerPrecedence(t *testing.T) {
	seedVault := t.TempDir()
	stateVault := t.TempDir()
	overrideVault := t.TempDir()
	dataDir := t.TempDir()

	m := NewManager(dataDir, seedVault)

	// Seed only.
	if got, err := m.Path(); err != nil || got != seedVault {
		t.Errorf("Path() = %q, %v; want the config seed %q", got, err, seedVault)
	}

	// State file beats the seed.
	if _, err := m.SetVault(stateVault); err != nil {
		t.Fatalf("SetVault: %v", err)
	}
	if got, err := m.Path(); err != nil || got != stateVault {
		t.Errorf("Path() = %q, %v; want the state file vault %q", got, err, stateVault)
	}

	// Override beats everything.
	m.SetOverride(overrideVault)
	if got, err := m.Path(); err != nil || got != overrideVault {
		t.Errorf("Path() = %q, %v; want the override %q", got, err, overrideVault)
	}

	// Clearing the override falls back to the state file.
	m.SetOverride("")
	if got, err := m.Path(); err != nil || got != stateVault {
		t.Errorf("Path() after clearing override = %q, %v; want %q", got, err, stateVault)
	}
}

func TestManagerSetVaultValidation(t *testing.T) {
	dataDir := t.TempDir()
	file := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		wantInvalid bool
		wantMissing bool
	}{
		{name: "empty", path: "", wantInvalid: true},
		{name: "relative", path: "vaults/personal", wantInvalid: true},
		{name: "nonexistent", path: filepath.Join(dataDir, "nope"), wantMissing: true},
		{name: "not a directory", path: file, wantInvalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(dataDir, "")
			_, err := m.SetVault(tt.path)
			if err == nil {
				t.Fatalf("SetVault(%q) succeeded, want error", tt.path)
			}
			if tt.wantInvalid && !errors.Is(err, ErrInvalidPath) {
				t.Errorf("error = %v, want ErrInvalidPath", err)
			}
			if tt.wantMissing && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("error = %v, want fs.ErrNotExist", err)
			}
			// A failed SetVault must not leave state behind.
			if _, err := m.Path(); !errors.Is(err, ErrNotConfigured) {
				t.Errorf("Path() after failed SetVault = %v, want ErrNotConfigured", err)
			}
		})
	}
}

// TestManagerStateReloadPropagation covers the daemon→MCP-server handoff: two
// managers sharing a data dir see each other's SetVault immediately, because
// the state file is re-read on every call.
func TestManagerStateReloadPropagation(t *testing.T) {
	dataDir := t.TempDir()
	firstVault := t.TempDir()
	secondVault := t.TempDir()

	daemonMgr := NewManager(dataDir, "")
	mcpMgr := NewManager(dataDir, "") // simulates the axios-obsidian process

	if _, err := mcpMgr.Vault(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("mcp manager error = %v, want ErrNotConfigured before any SetVault", err)
	}

	if _, err := daemonMgr.SetVault(firstVault); err != nil {
		t.Fatalf("SetVault(first): %v", err)
	}
	if got, err := mcpMgr.Path(); err != nil || got != firstVault {
		t.Errorf("mcp Path() = %q, %v; want %q without restart", got, err, firstVault)
	}

	// Switching vaults propagates too.
	if _, err := daemonMgr.SetVault(secondVault); err != nil {
		t.Fatalf("SetVault(second): %v", err)
	}
	if got, err := mcpMgr.Path(); err != nil || got != secondVault {
		t.Errorf("mcp Path() after switch = %q, %v; want %q", got, err, secondVault)
	}
	if v, err := mcpMgr.Vault(); err != nil || v.Root() != secondVault {
		t.Errorf("mcp Vault() = %v, %v; want root %q", v, err, secondVault)
	}
}

func TestManagerCorruptStateFile(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, stateFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	m := NewManager(dataDir, t.TempDir())
	if _, err := m.Path(); err == nil || !strings.Contains(err.Error(), "parse vault state") {
		t.Errorf("Path() error = %v, want a parse error for the corrupt state file", err)
	}

	// SetVault rewrites the state file and recovers.
	fresh := t.TempDir()
	if _, err := m.SetVault(fresh); err != nil {
		t.Fatalf("SetVault: %v", err)
	}
	if got, err := m.Path(); err != nil || got != fresh {
		t.Errorf("Path() after recovery = %q, %v; want %q", got, err, fresh)
	}
}

func TestManagerSeedState(t *testing.T) {
	t.Run("writes the seed for other processes", func(t *testing.T) {
		dataDir := t.TempDir()
		seedVault := t.TempDir()

		m := NewManager(dataDir, seedVault)
		if err := m.SeedState(); err != nil {
			t.Fatalf("SeedState: %v", err)
		}
		// A seedless manager on the same data dir (the MCP server) sees it.
		other := NewManager(dataDir, "")
		if got, err := other.Path(); err != nil || got != seedVault {
			t.Errorf("other Path() = %q, %v; want the seeded vault %q", got, err, seedVault)
		}
	})

	t.Run("never overwrites existing state", func(t *testing.T) {
		dataDir := t.TempDir()
		chosen := t.TempDir()
		seedVault := t.TempDir()

		m := NewManager(dataDir, seedVault)
		if _, err := m.SetVault(chosen); err != nil {
			t.Fatalf("SetVault: %v", err)
		}
		if err := m.SeedState(); err != nil {
			t.Fatalf("SeedState: %v", err)
		}
		if got, err := m.Path(); err != nil || got != chosen {
			t.Errorf("Path() = %q, %v; the user's choice must survive SeedState", got, err)
		}
	})

	t.Run("empty seed is a no-op", func(t *testing.T) {
		dataDir := t.TempDir()
		m := NewManager(dataDir, "")
		if err := m.SeedState(); err != nil {
			t.Fatalf("SeedState: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dataDir, stateFileName)); !errors.Is(err, fs.ErrNotExist) {
			t.Error("SeedState with an empty seed wrote a state file")
		}
	})

	t.Run("invalid seed reports an error", func(t *testing.T) {
		dataDir := t.TempDir()
		m := NewManager(dataDir, filepath.Join(dataDir, "missing-vault"))
		if err := m.SeedState(); err == nil {
			t.Error("SeedState with a nonexistent seed succeeded, want error")
		}
	})
}
