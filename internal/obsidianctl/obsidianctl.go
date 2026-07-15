// Package obsidianctl provides direct filesystem access to an Obsidian vault —
// a plain directory of Markdown notes. It backs the axios-obsidian MCP server
// and the daemon's /api/obsidian REST handlers so vault logic lives in one
// place; the Obsidian app itself is never required.
//
// Every note path is vault-relative and validated before it touches the
// filesystem: absolute paths, option-like names (leading "-"), backslashes,
// and ".." segments are rejected, the cleaned path is containment-checked
// against the vault root, and the deepest existing ancestor is resolved with
// filepath.EvalSymlinks so a symlink inside the vault cannot smuggle an
// operation outside it. Dot-prefixed files and directories (.obsidian/,
// .trash/, ...) are hidden from listing and search and refused for
// read/write/delete. Read/write/append/delete operate on .md files only;
// ".md" is appended automatically when the caller omits it.
package obsidianctl

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// ErrInvalidPath tags every path-validation failure (traversal, hidden names,
// non-vault-relative paths, ...) so API layers can map them to a 400 without
// string matching.
var ErrInvalidPath = errors.New("invalid path")

const (
	// maxSearchableSize caps how large a note Search will scan (1 MiB);
	// bigger files are skipped entirely.
	maxSearchableSize = 1 << 20
	// snippetLength is the approximate byte length of a search-hit snippet.
	snippetLength = 160
	// defaultSearchLimit and maxSearchLimit clamp the Search hit count.
	defaultSearchLimit = 20
	maxSearchLimit     = 100
)

// inlineTagPattern matches Obsidian inline tags in a note body, e.g. "#work"
// or "#projects/axios". The leading "#" is stripped before tags are returned.
var inlineTagPattern = regexp.MustCompile(`#[A-Za-z0-9_/-]+`)

// --- Vault data types ---

// NoteEntry is one row in a ListNotes result: a Markdown note or a folder
// (IsFolder) so a UI can tree-browse the vault.
type NoteEntry struct {
	Path     string `json:"path"` // vault-relative, "/"-separated
	Name     string `json:"name"`
	IsFolder bool   `json:"is_folder"`
	Size     int64  `json:"size"` // bytes; 0 for folders
	Modified string `json:"modified"`
}

// Note is the full content and metadata of one note.
type Note struct {
	Path string `json:"path"` // normalized vault-relative path (".md" included)
	// Content is the raw note text including any frontmatter block.
	Content string `json:"content"`
	// Frontmatter is the parsed leading "---" YAML block; nil when the note
	// has none or the block is malformed.
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
	// Tags merges frontmatter tags with inline "#tag" tokens from the body,
	// deduplicated (case-insensitive), "#" stripped, sorted.
	Tags     []string `json:"tags"`
	Size     int64    `json:"size"`
	Modified string   `json:"modified"`
}

// SearchHit is one Search result.
type SearchHit struct {
	Path string `json:"path"`
	Name string `json:"name"`
	// Snippet is ≈160 characters centered on the first content match
	// (filename-only matches show the start of the note), whitespace-collapsed.
	Snippet  string `json:"snippet"`
	Modified string `json:"modified"`
}

// VaultInfo summarizes a vault: note/folder counts and the total size of all
// visible notes in bytes.
type VaultInfo struct {
	Root           string `json:"root"`
	Name           string `json:"name"`
	Notes          int    `json:"notes"`
	Folders        int    `json:"folders"`
	TotalSizeBytes int64  `json:"total_size_bytes"`
}

// --- Vault ---

// Vault is an Obsidian vault rooted at an absolute directory. All operations
// take vault-relative note paths and enforce containment within the root.
type Vault struct {
	root string
}

// Open validates root (absolute path, existing directory) and returns a
// Vault. Validation failures carry ErrInvalidPath; a missing directory keeps
// its fs.ErrNotExist identity.
func Open(root string) (*Vault, error) {
	if root == "" {
		return nil, fmt.Errorf("%w: vault path must not be empty", ErrInvalidPath)
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("%w: vault path %q must be absolute", ErrInvalidPath, root)
	}
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("vault %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: vault %s is not a directory", ErrInvalidPath, root)
	}
	return &Vault{root: root}, nil
}

// Root returns the vault's absolute root directory.
func (v *Vault) Root() string {
	return v.root
}

// LooksLikeVault reports whether the root contains a .obsidian/ directory —
// the marker the Obsidian app leaves in every real vault. A missing marker is
// not an error (any directory of .md files works); callers surface it as a
// warning so the UI can flag a probable typo.
func (v *Vault) LooksLikeVault() bool {
	info, err := os.Stat(filepath.Join(v.root, ".obsidian"))
	return err == nil && info.IsDir()
}

// --- Path validation ---

// resolve validates a vault-relative path and returns its absolute location
// inside the vault. wantMD appends ".md" when the caller omitted the
// extension (note operations); folder resolution passes wantMD=false.
func (v *Vault) resolve(relPath string, wantMD bool) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("%w: path must not be empty", ErrInvalidPath)
	}
	if strings.Contains(relPath, `\`) {
		return "", fmt.Errorf("%w: %q must not contain backslashes (use '/' separators)", ErrInvalidPath, relPath)
	}
	if strings.HasPrefix(relPath, "-") {
		return "", fmt.Errorf("%w: %q must not start with '-'", ErrInvalidPath, relPath)
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("%w: %q must be vault-relative, not absolute", ErrInvalidPath, relPath)
	}
	// Reject any ".." segment in the raw path — even ones Clean would remove.
	for _, seg := range strings.Split(relPath, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w: %q must not contain '..' segments", ErrInvalidPath, relPath)
		}
	}

	clean := filepath.Clean(relPath)
	if clean == "." {
		return "", fmt.Errorf("%w: %q does not name a file or folder inside the vault", ErrInvalidPath, relPath)
	}
	// Dot-prefixed names (.obsidian/, .trash/, .hidden.md) are reserved and
	// never accessible through the vault API.
	for _, seg := range strings.Split(clean, string(filepath.Separator)) {
		if strings.HasPrefix(seg, ".") {
			return "", fmt.Errorf("%w: %q is hidden — dot-prefixed files and folders are not accessible", ErrInvalidPath, relPath)
		}
	}

	if wantMD && !strings.EqualFold(filepath.Ext(clean), ".md") {
		clean += ".md"
	}

	abs := filepath.Join(v.root, clean)
	// Containment re-check: the segment checks above already make escapes
	// impossible, but verify anyway (defense in depth).
	if rel, err := filepath.Rel(v.root, abs); err != nil || escapes(rel) {
		return "", fmt.Errorf("%w: %q escapes the vault", ErrInvalidPath, relPath)
	}
	if err := v.checkSymlinks(abs); err != nil {
		return "", err
	}
	return abs, nil
}

// checkSymlinks resolves the deepest existing ancestor of abs (which may be
// abs itself) and verifies the resolved location still lives inside the
// resolved vault root, so a symlink inside the vault cannot escape it.
func (v *Vault) checkSymlinks(abs string) error {
	deepest := abs
	for {
		if _, err := os.Lstat(deepest); err == nil {
			break
		}
		parent := filepath.Dir(deepest)
		if parent == deepest {
			break
		}
		deepest = parent
	}
	resolved, err := filepath.EvalSymlinks(deepest)
	if err != nil {
		return fmt.Errorf("resolve symlinks under %s: %w", deepest, err)
	}
	// The root is resolved too so vaults under symlinked parents (e.g. /tmp
	// on macOS) compare correctly.
	resolvedRoot, err := filepath.EvalSymlinks(v.root)
	if err != nil {
		return fmt.Errorf("resolve vault root: %w", err)
	}
	if rel, err := filepath.Rel(resolvedRoot, resolved); err != nil || escapes(rel) {
		return fmt.Errorf("%w: target resolves outside the vault (symlink escape)", ErrInvalidPath)
	}
	return nil
}

// escapes reports whether a filepath.Rel result points outside its base.
func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// isHiddenName reports whether a directory entry is dot-prefixed and thus
// hidden from listing and search.
func isHiddenName(name string) bool {
	return strings.HasPrefix(name, ".")
}

// isMarkdown reports whether a file name has a .md extension (any case).
func isMarkdown(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".md")
}

// --- Note operations ---

// ListNotes lists the Markdown notes and folders under a vault-relative
// folder ("" = the vault root), sorted by path. Hidden (dot-prefixed) entries
// are skipped; recursive descends into subfolders.
func (v *Vault) ListNotes(folder string, recursive bool) ([]NoteEntry, error) {
	dir := v.root
	if folder != "" {
		abs, err := v.resolve(folder, false)
		if err != nil {
			return nil, err
		}
		dir = abs
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("folder %s: %w", folder, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %q is not a folder", ErrInvalidPath, folder)
	}

	entries := []NoteEntry{}
	appendEntry := func(p string, d fs.DirEntry) {
		if e, ok := v.entryFor(p, d); ok {
			entries = append(entries, e)
		}
	}

	if recursive {
		err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == dir {
				return nil
			}
			if isHiddenName(d.Name()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			appendEntry(p, d)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("list folder %s: %w", folder, err)
		}
	} else {
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("list folder %s: %w", folder, err)
		}
		for _, d := range dirEntries {
			if isHiddenName(d.Name()) {
				continue
			}
			appendEntry(filepath.Join(dir, d.Name()), d)
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// entryFor builds a NoteEntry for a directory entry; non-Markdown files are
// skipped (ok=false).
func (v *Vault) entryFor(p string, d fs.DirEntry) (NoteEntry, bool) {
	if !d.IsDir() && !isMarkdown(d.Name()) {
		return NoteEntry{}, false
	}
	info, err := d.Info()
	if err != nil {
		return NoteEntry{}, false
	}
	rel, err := filepath.Rel(v.root, p)
	if err != nil {
		return NoteEntry{}, false
	}
	e := NoteEntry{
		Path:     filepath.ToSlash(rel),
		Name:     d.Name(),
		IsFolder: d.IsDir(),
		Modified: info.ModTime().UTC().Format(time.RFC3339),
	}
	if !d.IsDir() {
		e.Size = info.Size()
	}
	return e, true
}

// ReadNote reads a note and returns its content plus parsed frontmatter and
// tags. A malformed frontmatter block never fails the read — the note is
// returned with Frontmatter nil.
func (v *Vault) ReadNote(relPath string) (*Note, error) {
	abs, err := v.resolve(relPath, true)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("note %s: %w", relPath, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %q is a folder, not a note", ErrInvalidPath, relPath)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read note %s: %w", relPath, err)
	}

	content := string(data)
	frontmatter, body := parseFrontmatter(content)
	rel, err := filepath.Rel(v.root, abs)
	if err != nil {
		return nil, fmt.Errorf("relativize note path: %w", err)
	}
	return &Note{
		Path:        filepath.ToSlash(rel),
		Content:     content,
		Frontmatter: frontmatter,
		Tags:        extractTags(frontmatter, body),
		Size:        info.Size(),
		Modified:    info.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// WriteNote creates or replaces a note, creating parent folders inside the
// vault as needed. With overwrite=false an existing note is a clear error
// (fs.ErrExist) instead of silent data loss.
func (v *Vault) WriteNote(relPath, content string, overwrite bool) error {
	abs, err := v.resolve(relPath, true)
	if err != nil {
		return err
	}
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return fmt.Errorf("%w: %q is a folder, not a note", ErrInvalidPath, relPath)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("create parent folders for %s: %w", relPath, err)
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	f, err := os.OpenFile(abs, flags, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("note %s already exists (set overwrite to replace it): %w", relPath, err)
		}
		return fmt.Errorf("write note %s: %w", relPath, err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return fmt.Errorf("write note %s: %w", relPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write note %s: %w", relPath, err)
	}
	return nil
}

// AppendNote appends a block to a note, creating the note when it does not
// exist yet. Exactly one blank line separates the existing content from the
// appended block, regardless of how many trailing newlines the note had.
func (v *Vault) AppendNote(relPath, content string) error {
	abs, err := v.resolve(relPath, true)
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return v.WriteNote(relPath, content, false)
	}
	if err != nil {
		return fmt.Errorf("append note %s: %w", relPath, err)
	}

	final := content
	if trimmed := strings.TrimRight(string(existing), "\r\n"); trimmed != "" {
		final = trimmed + "\n\n" + content
	}
	if err := os.WriteFile(abs, []byte(final), 0o644); err != nil {
		return fmt.Errorf("append note %s: %w", relPath, err)
	}
	return nil
}

// DeleteNote deletes a single note. Folders are never deleted.
func (v *Vault) DeleteNote(relPath string) error {
	abs, err := v.resolve(relPath, true)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("note %s: %w", relPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: %q is a folder — only notes can be deleted", ErrInvalidPath, relPath)
	}
	if err := os.Remove(abs); err != nil {
		return fmt.Errorf("delete note %s: %w", relPath, err)
	}
	return nil
}

// Search finds notes by case-insensitive substring over filename + content,
// optionally restricted to notes carrying a tag (a leading "#" on the tag is
// ignored). With an empty query the tag alone filters. limit defaults to 20
// and is clamped to [1, 100]; hidden directories are skipped entirely and
// files over 1 MiB are never scanned.
func (v *Vault) Search(query, tag string, limit int) ([]SearchHit, error) {
	query = strings.TrimSpace(query)
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "#")
	if query == "" && tag == "" {
		return nil, fmt.Errorf("search needs a query or a tag")
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	lowerQuery := strings.ToLower(query)

	hits := []SearchHit{}
	err := filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if len(hits) >= limit {
			return filepath.SkipAll
		}
		if p == v.root {
			return nil
		}
		if isHiddenName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !isMarkdown(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxSearchableSize {
			return nil
		}

		nameMatch := lowerQuery != "" && strings.Contains(strings.ToLower(d.Name()), lowerQuery)
		data, err := os.ReadFile(p)
		if err != nil {
			return nil // unreadable note — skip it, keep searching
		}
		content := string(data)
		contentIdx := -1
		if lowerQuery != "" {
			contentIdx = strings.Index(strings.ToLower(content), lowerQuery)
		}
		if lowerQuery != "" && !nameMatch && contentIdx < 0 {
			return nil
		}
		if tag != "" && !noteHasTag(content, tag) {
			return nil
		}

		rel, err := filepath.Rel(v.root, p)
		if err != nil {
			return nil
		}
		hits = append(hits, SearchHit{
			Path:     filepath.ToSlash(rel),
			Name:     d.Name(),
			Snippet:  makeSnippet(content, contentIdx, len(query)),
			Modified: info.ModTime().UTC().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("search vault: %w", err)
	}
	return hits, nil
}

// Info scans the vault and returns note/folder counts plus the total size of
// all visible notes. Hidden directories are excluded from every figure.
func (v *Vault) Info() (*VaultInfo, error) {
	vi := &VaultInfo{Root: v.root, Name: filepath.Base(v.root)}
	err := filepath.WalkDir(v.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == v.root {
			return nil
		}
		if isHiddenName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			vi.Folders++
			return nil
		}
		if !isMarkdown(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		vi.Notes++
		vi.TotalSizeBytes += info.Size()
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan vault: %w", err)
	}
	return vi, nil
}

// --- Frontmatter and tags ---

// parseFrontmatter splits a leading "---\n...\n---" YAML block off a note.
// It returns the parsed frontmatter (nil when absent or malformed) and the
// body that follows the block (the full content when there is no block).
func parseFrontmatter(content string) (map[string]any, string) {
	after, found := strings.CutPrefix(content, "---\n")
	if !found {
		after, found = strings.CutPrefix(content, "---\r\n")
	}
	if !found {
		return nil, content
	}

	// Find the closing "---" on a line of its own (trailing \r tolerated).
	offset := 0
	for {
		lineEnd := strings.IndexByte(after[offset:], '\n')
		line := after[offset:]
		if lineEnd >= 0 {
			line = after[offset : offset+lineEnd]
		}
		if strings.TrimRight(line, "\r") == "---" {
			body := ""
			if lineEnd >= 0 {
				body = after[offset+lineEnd+1:]
			}
			var frontmatter map[string]any
			if err := yaml.Unmarshal([]byte(after[:offset]), &frontmatter); err != nil {
				return nil, body // malformed frontmatter — note still usable
			}
			return frontmatter, body
		}
		if lineEnd < 0 {
			// No closing delimiter: not a frontmatter block at all.
			return nil, content
		}
		offset += lineEnd + 1
	}
}

// extractTags merges frontmatter tags (a string or a list) with inline
// "#tag" tokens from the body. Tags are deduplicated case-insensitively
// (first spelling wins), stripped of a leading "#", and returned sorted.
func extractTags(frontmatter map[string]any, body string) []string {
	var raw []string
	switch v := frontmatter["tags"].(type) {
	case string:
		raw = append(raw, splitTagList(v)...)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				raw = append(raw, splitTagList(s)...)
			}
		}
	}
	for _, m := range inlineTagPattern.FindAllString(body, -1) {
		raw = append(raw, m)
	}

	seen := make(map[string]bool, len(raw))
	tags := []string{}
	for _, t := range raw {
		t = strings.TrimPrefix(strings.TrimSpace(t), "#")
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

// splitTagList splits frontmatter tag strings written as "work, ideas" or
// "work ideas" into individual tags.
func splitTagList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
}

// noteHasTag reports whether a note's content carries a tag (frontmatter or
// inline), compared case-insensitively.
func noteHasTag(content, tag string) bool {
	frontmatter, body := parseFrontmatter(content)
	for _, t := range extractTags(frontmatter, body) {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// makeSnippet returns ≈snippetLength bytes of content centered on the first
// match (matchIdx < 0 → the start of the note), trimmed to rune boundaries
// and whitespace-collapsed so hits render as a single line.
func makeSnippet(content string, matchIdx, matchLen int) string {
	if matchIdx < 0 || matchIdx > len(content) {
		matchIdx, matchLen = 0, 0
	}
	start := matchIdx - (snippetLength-matchLen)/2
	if start+snippetLength > len(content) {
		start = len(content) - snippetLength
	}
	if start < 0 {
		start = 0
	}
	end := start + snippetLength
	if end > len(content) {
		end = len(content)
	}
	// Never cut a UTF-8 rune in half.
	for start > 0 && !utf8.RuneStart(content[start]) {
		start--
	}
	for end < len(content) && !utf8.RuneStart(content[end]) {
		end++
	}
	return strings.Join(strings.Fields(content[start:end]), " ")
}
