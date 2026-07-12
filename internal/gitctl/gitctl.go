// Package gitctl wraps the git CLI for repository inspection (status, log,
// diff, branches, show, remotes) and mutation (pull, clone, checkout, commit,
// push). It backs the axios-git MCP server so git logic lives in one place.
//
// Every argument that reaches a git command line is validated first: repo and
// directory paths must be absolute (a leading "~" is expanded), ref/branch/
// remote names must match a conservative pattern that blocks option injection
// and ".." sequences, and clone URLs are restricted to https://, ssh://, and
// scp-style git@host:path — which shuts out git's command-executing
// pseudo-transports such as ext:: and fd::.
package gitctl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Command execution is injected behind package-level function variables so
// tests can fake git CLI output without touching a real repository. The
// context carries the per-operation timeout.
var (
	// lookGit reports whether the git CLI is on PATH.
	lookGit = func() error {
		_, err := exec.LookPath("git")
		return err
	}
	// runGit executes `git` with args under ctx and returns combined
	// stdout+stderr.
	runGit = func(ctx context.Context, args ...string) ([]byte, error) {
		return newGitCmd(ctx, args...).CombinedOutput()
	}
)

// newGitCmd builds the exec.Cmd for a git invocation. GIT_TERMINAL_PROMPT=0
// and GIT_ASKPASS=true make git fail fast instead of hanging on an
// interactive credential prompt.
func newGitCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=true")
	return cmd
}

const (
	// localTimeout bounds repository-local operations (status, log, diff, ...).
	localTimeout = 30 * time.Second
	// networkTimeout bounds operations that talk to a remote (pull, clone, push).
	networkTimeout = 120 * time.Second
	// minLogCount and maxLogCount clamp the git_log commit count.
	minLogCount = 1
	maxLogCount = 50
	// maxRefLen caps ref/branch/remote names well past anything legitimate.
	maxRefLen = 255
)

// refPattern matches a conservative subset of git ref, branch, and remote
// names: letters, digits, dots, underscores, slashes, and hyphens. The first
// character must be alphanumeric, which blocks option injection (leading "-")
// as well as shell metacharacters and spaces. ".." is rejected separately.
var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// Clone URL allowlist. Everything outside these three shapes — especially
// ext::, fd::, file://, and anything starting with "-" — is rejected, which
// blocks git's command-executing pseudo-transports and option injection.
var (
	// httpsURLPattern matches https://host[:port]/path URLs without userinfo.
	httpsURLPattern = regexp.MustCompile(`^https://[A-Za-z0-9][A-Za-z0-9.-]*(:[0-9]+)?(/[A-Za-z0-9._/-]*)?$`)
	// sshURLPattern matches ssh://[user@]host[:port]/path URLs.
	sshURLPattern = regexp.MustCompile(`^ssh://([A-Za-z0-9._-]+@)?[A-Za-z0-9][A-Za-z0-9.-]*(:[0-9]+)?(/[A-Za-z0-9._/-]*)?$`)
	// scpURLPattern matches scp-style git@host:path[.git] URLs.
	scpURLPattern = regexp.MustCompile(`^git@[A-Za-z0-9][A-Za-z0-9.-]*:/?[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

// Patterns extracting upstream divergence counts from the porcelain branch
// header, e.g. "## main...origin/main [ahead 1, behind 2]".
var (
	aheadPattern  = regexp.MustCompile(`ahead ([0-9]+)`)
	behindPattern = regexp.MustCompile(`behind ([0-9]+)`)
)

// --- Git data types ---

// RepoStatus is the parsed output of `git status --porcelain=v1 -b`.
type RepoStatus struct {
	Branch    string   `json:"branch"`
	Ahead     int      `json:"ahead"`
	Behind    int      `json:"behind"`
	Staged    []string `json:"staged"`
	Modified  []string `json:"modified"`
	Untracked []string `json:"untracked"`
}

// Commit is one entry from `git log`.
type Commit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

// Branch is one entry from `git branch -a`.
type Branch struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	Remote  bool   `json:"remote"`
}

// Remote is one configured remote with its fetch/push URLs deduped from
// `git remote -v`.
type Remote struct {
	Name     string `json:"name"`
	FetchURL string `json:"fetch_url,omitempty"`
	PushURL  string `json:"push_url,omitempty"`
}

// --- Validation ---

// Available checks whether the git CLI is installed.
func Available() error {
	if err := lookGit(); err != nil {
		return fmt.Errorf("git CLI not found in PATH: %w", err)
	}
	return nil
}

// ValidatePath expands a leading "~" to the user's home directory and
// requires the result to be an absolute path. Option-like paths (leading "-")
// are rejected before they can reach an exec'd command line. The cleaned
// absolute path is returned.
func ValidatePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	if strings.HasPrefix(path, "-") {
		return "", fmt.Errorf("invalid path %q: must not start with '-'", path)
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("invalid path %q: must be absolute", path)
	}
	return filepath.Clean(path), nil
}

// validateRepo validates a repository path and requires it to be an existing
// directory before any git command runs against it.
func validateRepo(repo string) (string, error) {
	path, err := ValidatePath(repo)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("repo %s: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo %s is not a directory", path)
	}
	return path, nil
}

// ValidateRef checks that a ref, branch, or remote name is a plausible git
// name and rejects option injection (leading "-"), ".." sequences, shell
// metacharacters, and spaces.
func ValidateRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("ref must not be empty")
	}
	if len(ref) > maxRefLen {
		return fmt.Errorf("ref exceeds %d characters", maxRefLen)
	}
	if !refPattern.MatchString(ref) {
		return fmt.Errorf("invalid ref %q: only letters, digits, '.', '_', '/' and '-' are allowed, starting with a letter or digit", ref)
	}
	if strings.Contains(ref, "..") {
		return fmt.Errorf("invalid ref %q: '..' sequences are not allowed", ref)
	}
	return nil
}

// ValidateCloneURL restricts clone URLs to https://, ssh://, and scp-style
// git@host:path. Everything else — file://, ext::, fd::, plain http://, and
// option-like strings — is rejected.
func ValidateCloneURL(url string) error {
	if url == "" {
		return fmt.Errorf("url must not be empty")
	}
	if httpsURLPattern.MatchString(url) || sshURLPattern.MatchString(url) || scpURLPattern.MatchString(url) {
		return nil
	}
	return fmt.Errorf("invalid clone URL %q: only https://host/path, ssh://host/path, or git@host:path URLs are allowed", url)
}

// validateFile checks a repository-relative file path passed to git diff.
// The "--" separator already stops option parsing; this adds an explicit
// guard against option-like and directory-escaping paths.
func validateFile(file string) error {
	if file == "" {
		return fmt.Errorf("file must not be empty")
	}
	if strings.HasPrefix(file, "-") {
		return fmt.Errorf("invalid file %q: must not start with '-'", file)
	}
	if strings.Contains(file, "..") {
		return fmt.Errorf("invalid file %q: '..' sequences are not allowed", file)
	}
	return nil
}

// --- Execution helpers ---

// gitLocal runs git with args under the local-operation timeout.
func gitLocal(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), localTimeout)
	defer cancel()
	return runGit(ctx, args...)
}

// gitNetwork runs git with args under the longer network-operation timeout.
func gitNetwork(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), networkTimeout)
	defer cancel()
	return runGit(ctx, args...)
}

// cmdError formats a failed git invocation, preferring git's own output and
// falling back to the exec error when git produced none.
func cmdError(op string, out []byte, err error) error {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return fmt.Errorf("%s failed: %w", op, err)
	}
	return fmt.Errorf("%s failed: %s", op, msg)
}

// --- Read-only operations ---

// Status runs `git status --porcelain=v1 -b` and parses the branch header,
// upstream divergence, and staged/modified/untracked file lists.
func Status(repo string) (*RepoStatus, error) {
	if err := Available(); err != nil {
		return nil, err
	}
	path, err := validateRepo(repo)
	if err != nil {
		return nil, err
	}

	out, err := gitLocal("-C", path, "status", "--porcelain=v1", "-b")
	if err != nil {
		return nil, cmdError("git status", out, err)
	}
	return parseStatus(string(out)), nil
}

// parseStatus parses porcelain v1 output with a branch header line.
func parseStatus(out string) *RepoStatus {
	st := &RepoStatus{Staged: []string{}, Modified: []string{}, Untracked: []string{}}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "## ") {
			parseStatusHeader(strings.TrimPrefix(line, "## "), st)
			continue
		}
		// Entries are "XY <path>": X is the index state, Y the worktree state.
		if len(line) < 4 {
			continue
		}
		x, y, path := line[0], line[1], line[3:]
		if x == '?' && y == '?' {
			st.Untracked = append(st.Untracked, path)
			continue
		}
		if x != ' ' && x != '?' {
			st.Staged = append(st.Staged, path)
		}
		if y != ' ' && y != '?' {
			st.Modified = append(st.Modified, path)
		}
	}
	return st
}

// parseStatusHeader parses the "## ..." branch line, which is one of:
// "main", "main...origin/main [ahead 1, behind 2]", "HEAD (no branch)",
// or "No commits yet on main".
func parseStatusHeader(header string, st *RepoStatus) {
	if strings.HasPrefix(header, "No commits yet on ") {
		st.Branch = strings.TrimPrefix(header, "No commits yet on ")
		return
	}
	st.Branch = header
	if i := strings.Index(header, "..."); i >= 0 {
		st.Branch = header[:i]
		if m := aheadPattern.FindStringSubmatch(header[i:]); m != nil {
			st.Ahead, _ = strconv.Atoi(m[1])
		}
		if m := behindPattern.FindStringSubmatch(header[i:]); m != nil {
			st.Behind, _ = strconv.Atoi(m[1])
		}
	}
}

// Log runs `git log -n <count>` with a pipe-separated pretty format and
// parses the commits. count is clamped to [1, 50].
func Log(repo string, count int) ([]Commit, error) {
	if err := Available(); err != nil {
		return nil, err
	}
	path, err := validateRepo(repo)
	if err != nil {
		return nil, err
	}
	if count < minLogCount {
		count = minLogCount
	}
	if count > maxLogCount {
		count = maxLogCount
	}

	out, err := gitLocal("-C", path, "log", "-n", strconv.Itoa(count),
		"--pretty=format:%H|%an|%ad|%s", "--date=iso")
	if err != nil {
		return nil, cmdError("git log", out, err)
	}
	return parseLog(string(out)), nil
}

// parseLog parses "hash|author|date|subject" lines; the subject may itself
// contain pipes, so the split is capped at four fields.
func parseLog(out string) []Commit {
	commits := []Commit{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue
		}
		commits = append(commits, Commit{Hash: parts[0], Author: parts[1], Date: parts[2], Subject: parts[3]})
	}
	return commits
}

// Diff returns a diffstat summary of pending changes (staged selects the
// index instead of the working tree), or the full unified patch for a single
// repository-relative file when file is non-empty.
func Diff(repo string, staged bool, file string) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}
	path, err := validateRepo(repo)
	if err != nil {
		return "", err
	}

	args := []string{"-C", path, "diff"}
	if staged {
		args = append(args, "--staged")
	}
	if file != "" {
		if err := validateFile(file); err != nil {
			return "", err
		}
		args = append(args, "--", file)
	} else {
		args = append(args, "--stat")
	}

	out, err := gitLocal(args...)
	if err != nil {
		return "", cmdError("git diff", out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Branches runs `git branch -a` with a refname format and parses local and
// remote branches, marking the current one.
func Branches(repo string) ([]Branch, error) {
	if err := Available(); err != nil {
		return nil, err
	}
	path, err := validateRepo(repo)
	if err != nil {
		return nil, err
	}

	out, err := gitLocal("-C", path, "branch", "-a", "--format=%(HEAD)%(refname)")
	if err != nil {
		return nil, cmdError("git branch", out, err)
	}
	return parseBranches(string(out)), nil
}

// parseBranches parses "%(HEAD)%(refname)" lines: the first character is "*"
// for the current branch, and the full refname distinguishes local heads from
// remote-tracking branches.
func parseBranches(out string) []Branch {
	branches := []Branch{}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		current, ref := line[0] == '*', line[1:]
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			branches = append(branches, Branch{Name: strings.TrimPrefix(ref, "refs/heads/"), Current: current})
		case strings.HasPrefix(ref, "refs/remotes/"):
			name := strings.TrimPrefix(ref, "refs/remotes/")
			// Skip symbolic entries like origin/HEAD that alias the remote's
			// default branch rather than naming a real branch.
			if strings.HasSuffix(name, "/HEAD") {
				continue
			}
			branches = append(branches, Branch{Name: name, Current: current, Remote: true})
		}
	}
	return branches
}

// Show runs `git show --stat` for a validated ref and returns the commit
// header plus diffstat.
func Show(repo, ref string) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}
	path, err := validateRepo(repo)
	if err != nil {
		return "", err
	}
	if err := ValidateRef(ref); err != nil {
		return "", err
	}

	out, err := gitLocal("-C", path, "show", "--stat",
		"--pretty=format:%H|%an|%ad|%s", "--date=iso", ref)
	if err != nil {
		return "", cmdError("git show", out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Remotes runs `git remote -v` and dedupes the output into one entry per
// remote with its fetch and push URLs, sorted by name for deterministic
// output.
func Remotes(repo string) ([]Remote, error) {
	if err := Available(); err != nil {
		return nil, err
	}
	path, err := validateRepo(repo)
	if err != nil {
		return nil, err
	}

	out, err := gitLocal("-C", path, "remote", "-v")
	if err != nil {
		return nil, cmdError("git remote", out, err)
	}
	return parseRemotes(string(out)), nil
}

// parseRemotes parses "name url (fetch|push)" lines.
func parseRemotes(out string) []Remote {
	byName := make(map[string]*Remote)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		name, url, kind := fields[0], fields[1], fields[2]
		r := byName[name]
		if r == nil {
			r = &Remote{Name: name}
			byName[name] = r
		}
		switch kind {
		case "(fetch)":
			r.FetchURL = url
		case "(push)":
			r.PushURL = url
		}
	}

	remotes := make([]Remote, 0, len(byName))
	for _, r := range byName {
		remotes = append(remotes, *r)
	}
	sort.Slice(remotes, func(i, j int) bool {
		return remotes[i].Name < remotes[j].Name
	})
	return remotes
}

// --- Mutating operations ---

// Pull runs `git pull --ff-only`: the branch only advances when it can
// fast-forward, so a pull never merges or rebases implicitly.
func Pull(repo string) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}
	path, err := validateRepo(repo)
	if err != nil {
		return "", err
	}

	out, err := gitNetwork("-C", path, "pull", "--ff-only")
	if err != nil {
		return "", cmdError("git pull", out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Clone runs `git clone -- <url> <dir>` for an allowlisted URL into a
// validated absolute directory.
func Clone(url, dir string) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}
	if err := ValidateCloneURL(url); err != nil {
		return "", err
	}
	path, err := ValidatePath(dir)
	if err != nil {
		return "", err
	}

	out, err := gitNetwork("clone", "--", url, path)
	if err != nil {
		return "", cmdError("git clone", out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Checkout switches to a branch; create=true creates it first (checkout -b).
func Checkout(repo, branch string, create bool) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}
	path, err := validateRepo(repo)
	if err != nil {
		return "", err
	}
	if err := ValidateRef(branch); err != nil {
		return "", err
	}

	args := []string{"-C", path, "checkout"}
	if create {
		args = append(args, "-b")
	}
	args = append(args, branch)

	out, err := gitLocal(args...)
	if err != nil {
		return "", cmdError("git checkout", out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CommitAll stages everything (`git add -A`), commits with the given message,
// and returns the new commit hash. Empty messages are rejected.
func CommitAll(repo, message string) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}
	path, err := validateRepo(repo)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("commit message must not be empty")
	}

	if out, err := gitLocal("-C", path, "add", "-A"); err != nil {
		return "", cmdError("git add", out, err)
	}
	if out, err := gitLocal("-C", path, "commit", "-m", message); err != nil {
		return "", cmdError("git commit", out, err)
	}
	out, err := gitLocal("-C", path, "rev-parse", "HEAD")
	if err != nil {
		return "", cmdError("git rev-parse", out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Push runs `git push`, pushing the current branch to its upstream by
// default. remote and branch optionally name the destination; branch requires
// remote. Force pushing is not supported by design.
func Push(repo, remote, branch string) (string, error) {
	if err := Available(); err != nil {
		return "", err
	}
	path, err := validateRepo(repo)
	if err != nil {
		return "", err
	}

	args := []string{"-C", path, "push"}
	if branch != "" && remote == "" {
		return "", fmt.Errorf("remote is required when branch is set")
	}
	if remote != "" {
		if err := ValidateRef(remote); err != nil {
			return "", err
		}
		args = append(args, remote)
		if branch != "" {
			if err := ValidateRef(branch); err != nil {
				return "", err
			}
			args = append(args, branch)
		}
	}

	out, err := gitNetwork(args...)
	if err != nil {
		return "", cmdError("git push", out, err)
	}
	return strings.TrimSpace(string(out)), nil
}
