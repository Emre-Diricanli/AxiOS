package obsidianctl

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// newTestVault opens a Vault over a fresh temp directory.
func newTestVault(t *testing.T) *Vault {
	t.Helper()
	v, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open temp vault: %v", err)
	}
	return v
}

// writeVaultFile creates a file (with parents) inside a vault root, bypassing
// the Vault API so tests can stage hidden and non-Markdown files too.
func writeVaultFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestOpen(t *testing.T) {
	root := t.TempDir()
	plainFile := filepath.Join(root, "file.md")
	if err := os.WriteFile(plainFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		wantErr     string
		wantInvalid bool
		wantMissing bool
	}{
		{name: "valid directory", path: root},
		{name: "empty", path: "", wantErr: "must not be empty", wantInvalid: true},
		{name: "relative", path: "some/dir", wantErr: "must be absolute", wantInvalid: true},
		{name: "missing", path: filepath.Join(root, "nope"), wantErr: "no such file", wantMissing: true},
		{name: "not a directory", path: plainFile, wantErr: "is not a directory", wantInvalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := Open(tt.path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Open(%q) error: %v", tt.path, err)
				}
				if v.Root() != filepath.Clean(tt.path) {
					t.Errorf("Root() = %q, want %q", v.Root(), tt.path)
				}
				return
			}
			if err == nil {
				t.Fatalf("Open(%q) succeeded, want error containing %q", tt.path, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if tt.wantInvalid && !errors.Is(err, ErrInvalidPath) {
				t.Errorf("error = %v, want ErrInvalidPath", err)
			}
			if tt.wantMissing && !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("error = %v, want fs.ErrNotExist", err)
			}
		})
	}
}

func TestLooksLikeVault(t *testing.T) {
	v := newTestVault(t)
	if v.LooksLikeVault() {
		t.Error("LooksLikeVault() = true for a bare directory")
	}
	if err := os.MkdirAll(filepath.Join(v.Root(), ".obsidian"), 0o755); err != nil {
		t.Fatalf("mkdir .obsidian: %v", err)
	}
	if !v.LooksLikeVault() {
		t.Error("LooksLikeVault() = false with a .obsidian/ directory present")
	}
}

// TestPathValidation drives the shared resolve logic through every operation
// with hostile and hidden paths — all must be refused with ErrInvalidPath.
func TestPathValidation(t *testing.T) {
	v := newTestVault(t)
	writeVaultFile(t, v.Root(), "real.md", "content")

	paths := []struct {
		name string
		path string
	}{
		{"empty", ""},
		{"parent traversal", "../x"},
		{"nested traversal", "a/../../x"},
		{"masked traversal", "a/../b.md"}, // any ".." segment is rejected, even harmless ones
		{"absolute", "/etc/passwd"},
		{"backslash", `a\b.md`},
		{"leading dash", "-flag.md"},
		{"dot only", "."},
		{"obsidian config dir", ".obsidian/app.json"},
		{"trash dir", ".trash/old.md"},
		{"hidden file", ".hidden.md"},
		{"hidden middle dir", "a/.git/x.md"},
	}
	ops := []struct {
		name string
		call func(path string) error
	}{
		{"ReadNote", func(p string) error { _, err := v.ReadNote(p); return err }},
		{"WriteNote", func(p string) error { return v.WriteNote(p, "x", true) }},
		{"AppendNote", func(p string) error { return v.AppendNote(p, "x") }},
		{"DeleteNote", func(p string) error { return v.DeleteNote(p) }},
	}
	for _, op := range ops {
		for _, tt := range paths {
			t.Run(op.name+"/"+tt.name, func(t *testing.T) {
				err := op.call(tt.path)
				if !errors.Is(err, ErrInvalidPath) {
					t.Errorf("%s(%q) error = %v, want ErrInvalidPath", op.name, tt.path, err)
				}
			})
		}
	}

	// ListNotes folder paths go through the same validation (minus ".md").
	for _, folder := range []string{"../x", "/abs", `a\b`, ".obsidian", "-f"} {
		if _, err := v.ListNotes(folder, false); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("ListNotes(%q) error = %v, want ErrInvalidPath", folder, err)
		}
	}
}

func TestSymlinkEscape(t *testing.T) {
	outside := t.TempDir()
	writeVaultFile(t, outside, "secret.md", "outside the vault")

	v := newTestVault(t)
	writeVaultFile(t, v.Root(), "Real/inside.md", "inside")

	// A symlinked directory pointing outside the vault.
	if err := os.Symlink(outside, filepath.Join(v.Root(), "escape")); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}
	// A symlinked file pointing outside the vault.
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(v.Root(), "leak.md")); err != nil {
		t.Fatalf("symlink file: %v", err)
	}
	// A benign symlink that stays inside the vault.
	if err := os.Symlink(filepath.Join(v.Root(), "Real"), filepath.Join(v.Root(), "alias")); err != nil {
		t.Fatalf("symlink alias: %v", err)
	}

	escapes := []struct {
		name string
		call func() error
	}{
		{"read through dir symlink", func() error { _, err := v.ReadNote("escape/secret.md"); return err }},
		{"read file symlink", func() error { _, err := v.ReadNote("leak.md"); return err }},
		{"write through dir symlink", func() error { return v.WriteNote("escape/new.md", "x", true) }},
		{"write new file under dir symlink", func() error { return v.WriteNote("escape/brand-new.md", "x", false) }},
		{"append through dir symlink", func() error { return v.AppendNote("escape/secret.md", "x") }},
		{"delete through dir symlink", func() error { return v.DeleteNote("escape/secret.md") }},
		{"delete file symlink", func() error { return v.DeleteNote("leak.md") }},
	}
	for _, tt := range escapes {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, ErrInvalidPath) {
				t.Errorf("error = %v, want ErrInvalidPath (symlink escape)", err)
			}
		})
	}

	// Symlinks that resolve inside the vault keep working.
	note, err := v.ReadNote("alias/inside.md")
	if err != nil {
		t.Fatalf("ReadNote through in-vault symlink: %v", err)
	}
	if note.Content != "inside" {
		t.Errorf("content = %q, want %q", note.Content, "inside")
	}
}

func TestReadNote(t *testing.T) {
	v := newTestVault(t)
	writeVaultFile(t, v.Root(), "full.md",
		"---\ntitle: Full Note\ntags:\n  - work\n  - \"#ideas\"\n---\nBody with #inline and #work/sub tags.\n")
	writeVaultFile(t, v.Root(), "stringtags.md", "---\ntags: alpha, beta\n---\nplain body\n")
	writeVaultFile(t, v.Root(), "broken.md", "---\ntags: [unclosed\n---\nStill #readable body.\n")
	writeVaultFile(t, v.Root(), "plain.md", "No frontmatter, just #plain text.\n")
	writeVaultFile(t, v.Root(), "Work/deep.md", "nested note\n")

	tests := []struct {
		name        string
		path        string
		wantPath    string
		wantContent string // "" = skip check
		wantFM      bool
		wantTags    []string
	}{
		{
			name:     "frontmatter list tags merge with inline tags",
			path:     "full.md",
			wantPath: "full.md",
			wantFM:   true,
			wantTags: []string{"ideas", "inline", "work", "work/sub"},
		},
		{
			name:     "frontmatter string tags",
			path:     "stringtags.md",
			wantPath: "stringtags.md",
			wantFM:   true,
			wantTags: []string{"alpha", "beta"},
		},
		{
			name:     "malformed frontmatter still returns the note",
			path:     "broken.md",
			wantPath: "broken.md",
			wantFM:   false,
			wantTags: []string{"readable"},
		},
		{
			name:        "no frontmatter",
			path:        "plain.md",
			wantPath:    "plain.md",
			wantContent: "No frontmatter, just #plain text.\n",
			wantFM:      false,
			wantTags:    []string{"plain"},
		},
		{
			name:     "md extension appended automatically",
			path:     "Work/deep",
			wantPath: "Work/deep.md",
			wantFM:   false,
			wantTags: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note, err := v.ReadNote(tt.path)
			if err != nil {
				t.Fatalf("ReadNote(%q): %v", tt.path, err)
			}
			if note.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", note.Path, tt.wantPath)
			}
			if tt.wantContent != "" && note.Content != tt.wantContent {
				t.Errorf("Content = %q, want %q", note.Content, tt.wantContent)
			}
			if tt.wantFM && note.Frontmatter == nil {
				t.Error("Frontmatter = nil, want parsed map")
			}
			if !tt.wantFM && note.Frontmatter != nil {
				t.Errorf("Frontmatter = %v, want nil", note.Frontmatter)
			}
			if !reflect.DeepEqual(note.Tags, tt.wantTags) {
				t.Errorf("Tags = %v, want %v", note.Tags, tt.wantTags)
			}
			if note.Size <= 0 || note.Modified == "" {
				t.Errorf("Size/Modified not populated: %d %q", note.Size, note.Modified)
			}
		})
	}

	t.Run("missing note", func(t *testing.T) {
		if _, err := v.ReadNote("nope.md"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("error = %v, want fs.ErrNotExist", err)
		}
	})
	t.Run("malformed frontmatter keeps whole content", func(t *testing.T) {
		note, err := v.ReadNote("broken.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		if !strings.HasPrefix(note.Content, "---\n") {
			t.Errorf("Content = %q, want the raw file including the broken block", note.Content)
		}
	})
}

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantFM   bool
		wantBody string
	}{
		{"simple block", "---\na: 1\n---\nbody\n", true, "body\n"},
		{"crlf block", "---\r\na: 1\r\n---\r\nbody", true, "body"},
		{"closing at eof", "---\na: 1\n---", true, ""},
		{"no block", "just text\n", false, "just text\n"},
		{"unterminated block", "---\na: 1\nno closing", false, "---\na: 1\nno closing"},
		{"malformed yaml", "---\n[broken\n---\nbody", false, "body"},
		{"empty block", "---\n---\nbody", false, "body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body := parseFrontmatter(tt.content)
			if tt.wantFM && fm == nil {
				t.Error("frontmatter = nil, want parsed map")
			}
			if !tt.wantFM && fm != nil {
				t.Errorf("frontmatter = %v, want nil", fm)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestExtractTagsDedup(t *testing.T) {
	fm := map[string]any{"tags": []any{"Work", "#work", "ideas"}}
	got := extractTags(fm, "body with #WORK and #ideas/sub")
	want := []string{"Work", "ideas", "ideas/sub"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("extractTags = %v, want %v (case-insensitive dedup, first spelling wins, sorted)", got, want)
	}
}

func TestWriteNote(t *testing.T) {
	v := newTestVault(t)

	t.Run("creates parents and appends md", func(t *testing.T) {
		if err := v.WriteNote("Deep/Nested/note", "hello", false); err != nil {
			t.Fatalf("WriteNote: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(v.Root(), "Deep", "Nested", "note.md"))
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(data) != "hello" {
			t.Errorf("content = %q, want %q", data, "hello")
		}
	})

	t.Run("existing without overwrite is a clear error", func(t *testing.T) {
		err := v.WriteNote("Deep/Nested/note.md", "clobber", false)
		if !errors.Is(err, fs.ErrExist) {
			t.Fatalf("error = %v, want fs.ErrExist", err)
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("error = %q, want it to mention the note already exists", err)
		}
	})

	t.Run("overwrite replaces content", func(t *testing.T) {
		if err := v.WriteNote("Deep/Nested/note.md", "v2", true); err != nil {
			t.Fatalf("WriteNote overwrite: %v", err)
		}
		note, err := v.ReadNote("Deep/Nested/note.md")
		if err != nil {
			t.Fatalf("ReadNote: %v", err)
		}
		if note.Content != "v2" {
			t.Errorf("content = %q, want %q", note.Content, "v2")
		}
	})

	t.Run("directory target refused", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(v.Root(), "dir.md"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := v.WriteNote("dir.md", "x", true); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("error = %v, want ErrInvalidPath for a folder target", err)
		}
	})
}

func TestAppendNote(t *testing.T) {
	tests := []struct {
		name     string
		existing string // "" with exists=false means the note is missing
		exists   bool
		appended string
		want     string
	}{
		{name: "creates missing note", appended: "first", want: "first"},
		{name: "single blank line separator", existing: "old\n", exists: true, appended: "new", want: "old\n\nnew"},
		{name: "collapses extra trailing newlines", existing: "old\n\n\n\n", exists: true, appended: "new", want: "old\n\nnew"},
		{name: "no trailing newline on old content", existing: "old", exists: true, appended: "new", want: "old\n\nnew"},
		{name: "empty existing file gets no leading blank", existing: "", exists: true, appended: "new", want: "new"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestVault(t)
			if tt.exists {
				writeVaultFile(t, v.Root(), "a.md", tt.existing)
			}
			if err := v.AppendNote("a.md", tt.appended); err != nil {
				t.Fatalf("AppendNote: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(v.Root(), "a.md"))
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("content = %q, want %q", data, tt.want)
			}
		})
	}
}

func TestDeleteNote(t *testing.T) {
	v := newTestVault(t)
	writeVaultFile(t, v.Root(), "gone.md", "x")
	if err := os.MkdirAll(filepath.Join(v.Root(), "folder.md"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := v.DeleteNote("gone"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	if _, err := os.Stat(filepath.Join(v.Root(), "gone.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("note still exists after DeleteNote")
	}
	if _, err := v.ReadNote("gone.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadNote after delete = %v, want fs.ErrNotExist", err)
	}
	if err := v.DeleteNote("gone.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("double delete error = %v, want fs.ErrNotExist", err)
	}
	if err := v.DeleteNote("folder.md"); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("folder delete error = %v, want ErrInvalidPath (files only)", err)
	}
}

func TestListNotes(t *testing.T) {
	v := newTestVault(t)
	writeVaultFile(t, v.Root(), "a.md", "root note")
	writeVaultFile(t, v.Root(), "b.txt", "not a note")
	writeVaultFile(t, v.Root(), "Work/c.md", "work note")
	writeVaultFile(t, v.Root(), "Work/Sub/d.md", "deep note")
	writeVaultFile(t, v.Root(), ".obsidian/app.json", "{}")
	writeVaultFile(t, v.Root(), ".trash/junk.md", "trashed")
	writeVaultFile(t, v.Root(), ".hidden.md", "hidden")

	paths := func(entries []NoteEntry) []string {
		got := make([]string, len(entries))
		for i, e := range entries {
			got[i] = e.Path
			if e.IsFolder {
				got[i] += "/"
			}
		}
		return got
	}

	tests := []struct {
		name      string
		folder    string
		recursive bool
		want      []string
	}{
		{"root non-recursive", "", false, []string{"Work/", "a.md"}},
		{"root recursive", "", true, []string{"Work/", "Work/Sub/", "Work/Sub/d.md", "Work/c.md", "a.md"}},
		{"subfolder non-recursive", "Work", false, []string{"Work/Sub/", "Work/c.md"}},
		{"subfolder recursive", "Work/Sub", true, []string{"Work/Sub/d.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := v.ListNotes(tt.folder, tt.recursive)
			if err != nil {
				t.Fatalf("ListNotes(%q, %v): %v", tt.folder, tt.recursive, err)
			}
			if got := paths(entries); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("entries = %v, want %v", got, tt.want)
			}
			for _, e := range entries {
				if e.Modified == "" || e.Name == "" {
					t.Errorf("entry %q missing name/modified: %+v", e.Path, e)
				}
				if !e.IsFolder && e.Size <= 0 {
					t.Errorf("note %q has size %d, want > 0", e.Path, e.Size)
				}
			}
		})
	}

	t.Run("missing folder", func(t *testing.T) {
		if _, err := v.ListNotes("Nope", false); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("error = %v, want fs.ErrNotExist", err)
		}
	})
	t.Run("folder is a file", func(t *testing.T) {
		if _, err := v.ListNotes("a.md", false); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("error = %v, want ErrInvalidPath", err)
		}
	})
}

func TestSearch(t *testing.T) {
	v := newTestVault(t)
	writeVaultFile(t, v.Root(), "alpha.md", "The quick brown fox jumps over the lazy dog.")
	writeVaultFile(t, v.Root(), "Notes/beta.md", "hello WORLD, this note is #work related")
	writeVaultFile(t, v.Root(), "gamma.md", "---\ntags:\n  - work\n---\nProject overview\n")
	writeVaultFile(t, v.Root(), ".trash/hit.md", "hello world in the trash")
	writeVaultFile(t, v.Root(), "big.md", strings.Repeat("x", maxSearchableSize)+" hello world")
	writeVaultFile(t, v.Root(), "plain.txt", "hello world but not markdown")

	t.Run("case-insensitive content match", func(t *testing.T) {
		hits, err := v.Search("world", "", 0)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(hits) != 1 || hits[0].Path != "Notes/beta.md" {
			t.Fatalf("hits = %+v, want only Notes/beta.md", hits)
		}
		if !strings.Contains(hits[0].Snippet, "WORLD") {
			t.Errorf("snippet = %q, want it to contain the match", hits[0].Snippet)
		}
		if hits[0].Name != "beta.md" || hits[0].Modified == "" {
			t.Errorf("hit metadata incomplete: %+v", hits[0])
		}
	})

	t.Run("filename match uses leading snippet", func(t *testing.T) {
		hits, err := v.Search("alpha", "", 0)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(hits) != 1 || hits[0].Path != "alpha.md" {
			t.Fatalf("hits = %+v, want only alpha.md", hits)
		}
		if !strings.HasPrefix(hits[0].Snippet, "The quick") {
			t.Errorf("snippet = %q, want the start of the note", hits[0].Snippet)
		}
	})

	t.Run("tag filter restricts hits", func(t *testing.T) {
		hits, err := v.Search("o", "work", 0)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		got := make([]string, len(hits))
		for i, h := range hits {
			got[i] = h.Path
		}
		want := []string{"Notes/beta.md", "gamma.md"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("hits = %v, want %v (frontmatter + inline tags)", got, want)
		}
	})

	t.Run("tag-only search with leading hash", func(t *testing.T) {
		hits, err := v.Search("", "#work", 0)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(hits) != 2 {
			t.Errorf("hits = %+v, want the two #work notes", hits)
		}
	})

	t.Run("empty query and tag rejected", func(t *testing.T) {
		if _, err := v.Search("", "", 0); err == nil {
			t.Error("Search with no query and no tag succeeded, want error")
		}
	})
}

func TestSearchLimitsAndSnippet(t *testing.T) {
	v := newTestVault(t)
	for i := 0; i < 25; i++ {
		writeVaultFile(t, v.Root(), strings.Repeat("m", i+1)+".md", "needle content")
	}

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"zero limit defaults to 20", 0, 20},
		{"negative limit defaults to 20", -3, 20},
		{"small limit caps hits", 2, 2},
		{"huge limit clamps to 100 (all 25 fit)", 1000, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits, err := v.Search("needle", "", tt.limit)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(hits) != tt.want {
				t.Errorf("len(hits) = %d, want %d", len(hits), tt.want)
			}
		})
	}

	t.Run("snippet centered on the first match", func(t *testing.T) {
		long := strings.Repeat("start ", 100) + "NEEDLE" + strings.Repeat(" finish", 100)
		writeVaultFile(t, v.Root(), "long.md", long)
		hits, err := v.Search("needle", "", 100)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		var snippet string
		for _, h := range hits {
			if h.Path == "long.md" {
				snippet = h.Snippet
			}
		}
		if snippet == "" {
			t.Fatal("long.md not found in hits")
		}
		if !strings.Contains(snippet, "NEEDLE") {
			t.Errorf("snippet = %q, want it centered on the match", snippet)
		}
		if strings.HasPrefix(snippet, "start start start start start") && !strings.Contains(snippet, "NEEDLE") {
			t.Errorf("snippet = %q, want a window around the match, not the file head", snippet)
		}
		if len(snippet) > snippetLength+8 {
			t.Errorf("len(snippet) = %d, want ≈%d", len(snippet), snippetLength)
		}
	})
}

func TestInfo(t *testing.T) {
	v := newTestVault(t)
	writeVaultFile(t, v.Root(), "a.md", "12345")
	writeVaultFile(t, v.Root(), "Work/b.md", "1234567890")
	writeVaultFile(t, v.Root(), "Work/Sub/c.md", "123")
	writeVaultFile(t, v.Root(), "Work/skip.txt", "not counted")
	writeVaultFile(t, v.Root(), ".obsidian/workspace.json", "{}")
	writeVaultFile(t, v.Root(), ".trash/old.md", "gone")

	info, err := v.Info()
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Root != v.Root() || info.Name != filepath.Base(v.Root()) {
		t.Errorf("Root/Name = %q/%q, want %q/%q", info.Root, info.Name, v.Root(), filepath.Base(v.Root()))
	}
	if info.Notes != 3 {
		t.Errorf("Notes = %d, want 3 (hidden and non-md excluded)", info.Notes)
	}
	if info.Folders != 2 {
		t.Errorf("Folders = %d, want 2 (Work, Work/Sub)", info.Folders)
	}
	if info.TotalSizeBytes != 18 {
		t.Errorf("TotalSizeBytes = %d, want 18", info.TotalSizeBytes)
	}
}
