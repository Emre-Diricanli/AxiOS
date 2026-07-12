package gitctl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// gitResult is one queued outcome for the fake git runner.
type gitResult struct {
	output string
	err    error
}

// fakeGit swaps the injected lookup/run functions for the duration of a test
// and records the args of every git invocation. Each call consumes the next
// queued result; the last one repeats once the queue runs out. The fake also
// asserts that every invocation carries a context deadline so no git command
// can hang forever.
func fakeGit(t *testing.T, lookErr error, results ...gitResult) *[][]string {
	t.Helper()

	origLook, origRun := lookGit, runGit
	t.Cleanup(func() {
		lookGit, runGit = origLook, origRun
	})

	var calls [][]string
	lookGit = func() error { return lookErr }
	runGit = func(ctx context.Context, args ...string) ([]byte, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Errorf("git invoked without a context deadline: %v", args)
		}
		calls = append(calls, args)
		r := gitResult{}
		if len(results) > 0 {
			r = results[0]
			if len(results) > 1 {
				results = results[1:]
			}
		}
		return []byte(r.output), r.err
	}
	return &calls
}

func TestStatus(t *testing.T) {
	repo := t.TempDir()
	plainFile := filepath.Join(repo, "plain.txt")
	if err := os.WriteFile(plainFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write plain file: %v", err)
	}

	tests := []struct {
		name    string
		repo    string
		lookErr error
		output  string
		runErr  error
		want    *RepoStatus
		wantErr string
	}{
		{
			name: "parses branch, divergence, and file states",
			repo: repo,
			output: "## main...origin/main [ahead 2, behind 1]\n" +
				"M  staged.go\n" +
				"A  added.go\n" +
				" M edited.go\n" +
				"MM both.go\n" +
				"D  removed.go\n" +
				"R  old.go -> new.go\n" +
				"?? notes.txt\n",
			want: &RepoStatus{
				Branch: "main", Ahead: 2, Behind: 1,
				Staged:    []string{"staged.go", "added.go", "both.go", "removed.go", "old.go -> new.go"},
				Modified:  []string{"edited.go", "both.go"},
				Untracked: []string{"notes.txt"},
			},
		},
		{
			name:   "ahead only",
			repo:   repo,
			output: "## dev...origin/dev [ahead 3]\n",
			want:   &RepoStatus{Branch: "dev", Ahead: 3, Staged: []string{}, Modified: []string{}, Untracked: []string{}},
		},
		{
			name:   "clean repo without upstream",
			repo:   repo,
			output: "## main\n",
			want:   &RepoStatus{Branch: "main", Staged: []string{}, Modified: []string{}, Untracked: []string{}},
		},
		{
			name:   "detached HEAD",
			repo:   repo,
			output: "## HEAD (no branch)\n",
			want:   &RepoStatus{Branch: "HEAD (no branch)", Staged: []string{}, Modified: []string{}, Untracked: []string{}},
		},
		{
			name:   "no commits yet",
			repo:   repo,
			output: "## No commits yet on main\n?? a.txt\n",
			want:   &RepoStatus{Branch: "main", Staged: []string{}, Modified: []string{}, Untracked: []string{"a.txt"}},
		},
		{
			name:    "missing repo",
			repo:    filepath.Join(repo, "missing"),
			wantErr: "no such file or directory",
		},
		{
			name:    "repo is a file",
			repo:    plainFile,
			wantErr: "is not a directory",
		},
		{
			name:    "relative repo path",
			repo:    "some/repo",
			wantErr: "must be absolute",
		},
		{
			name:    "option-like repo path",
			repo:    "-C/evil",
			wantErr: "must not start with '-'",
		},
		{
			name:    "empty repo path",
			repo:    "",
			wantErr: "path must not be empty",
		},
		{
			name:    "missing binary",
			repo:    repo,
			lookErr: errors.New("executable file not found in $PATH"),
			wantErr: "git CLI not found in PATH",
		},
		{
			name:    "command failure surfaces output",
			repo:    repo,
			output:  "fatal: not a git repository",
			runErr:  errors.New("exit status 128"),
			wantErr: "git status failed: fatal: not a git repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeGit(t, tt.lookErr, gitResult{tt.output, tt.runErr})

			got, err := Status(tt.repo)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Status() error = %v, want containing %q", err, tt.wantErr)
				}
				if tt.runErr == nil && len(*calls) != 0 {
					t.Errorf("git was executed for invalid input: %v", *calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Status() = %+v, want %+v", got, tt.want)
			}
			wantArgs := []string{"-C", tt.repo, "status", "--porcelain=v1", "-b"}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], wantArgs) {
				t.Errorf("git args = %v, want [%v]", *calls, wantArgs)
			}
		})
	}
}

func TestLog(t *testing.T) {
	repo := t.TempDir()

	tests := []struct {
		name      string
		count     int
		output    string
		runErr    error
		wantCount string
		want      []Commit
		wantErr   string
	}{
		{
			name:  "parses pipe-separated commits",
			count: 10,
			output: "aaa111|Alice|2026-07-01 10:00:00 +0000|feat: add thing\n" +
				"bbb222|Bob|2026-06-30 09:00:00 +0000|fix: remove bug\n",
			wantCount: "10",
			want: []Commit{
				{Hash: "aaa111", Author: "Alice", Date: "2026-07-01 10:00:00 +0000", Subject: "feat: add thing"},
				{Hash: "bbb222", Author: "Bob", Date: "2026-06-30 09:00:00 +0000", Subject: "fix: remove bug"},
			},
		},
		{
			name:      "subject keeps embedded pipes",
			count:     5,
			output:    "ccc333|Carol|2026-07-02|feat: a | b | c\n",
			wantCount: "5",
			want: []Commit{
				{Hash: "ccc333", Author: "Carol", Date: "2026-07-02", Subject: "feat: a | b | c"},
			},
		},
		{
			name:      "count below range clamps to 1",
			count:     0,
			output:    "",
			wantCount: "1",
			want:      []Commit{},
		},
		{
			name:      "negative count clamps to 1",
			count:     -7,
			output:    "",
			wantCount: "1",
			want:      []Commit{},
		},
		{
			name:      "count above range clamps to 50",
			count:     999,
			output:    "",
			wantCount: "50",
			want:      []Commit{},
		},
		{
			name:      "skips blank and malformed lines",
			count:     3,
			output:    "\nnot-a-commit-line\nddd444|Dan|2026-07-03|chore\n",
			wantCount: "3",
			want: []Commit{
				{Hash: "ddd444", Author: "Dan", Date: "2026-07-03", Subject: "chore"},
			},
		},
		{
			name:      "command failure surfaces output",
			count:     10,
			output:    "fatal: your current branch 'main' does not have any commits yet",
			runErr:    errors.New("exit status 128"),
			wantCount: "10",
			wantErr:   "git log failed: fatal: your current branch 'main' does not have any commits yet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeGit(t, nil, gitResult{tt.output, tt.runErr})

			got, err := Log(repo, tt.count)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Log() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Log() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Log() = %+v, want %+v", got, tt.want)
			}
			wantArgs := []string{"-C", repo, "log", "-n", tt.wantCount, "--pretty=format:%H|%an|%ad|%s", "--date=iso"}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], wantArgs) {
				t.Errorf("git args = %v, want [%v]", *calls, wantArgs)
			}
		})
	}
}

func TestDiff(t *testing.T) {
	repo := t.TempDir()

	tests := []struct {
		name     string
		staged   bool
		file     string
		output   string
		runErr   error
		wantArgs []string
		want     string
		wantErr  string
	}{
		{
			name:     "worktree diffstat",
			output:   " main.go | 4 ++--\n 1 file changed, 2 insertions(+), 2 deletions(-)\n",
			wantArgs: []string{"-C", repo, "diff", "--stat"},
			want:     "main.go | 4 ++--\n 1 file changed, 2 insertions(+), 2 deletions(-)",
		},
		{
			name:     "staged diffstat",
			staged:   true,
			output:   " main.go | 1 +\n",
			wantArgs: []string{"-C", repo, "diff", "--staged", "--stat"},
			want:     "main.go | 1 +",
		},
		{
			name:     "single file patch uses -- separator",
			file:     "cmd/main.go",
			output:   "diff --git a/cmd/main.go b/cmd/main.go\n",
			wantArgs: []string{"-C", repo, "diff", "--", "cmd/main.go"},
			want:     "diff --git a/cmd/main.go b/cmd/main.go",
		},
		{
			name:     "staged single file patch",
			staged:   true,
			file:     "cmd/main.go",
			output:   "diff --git a/cmd/main.go b/cmd/main.go\n",
			wantArgs: []string{"-C", repo, "diff", "--staged", "--", "cmd/main.go"},
			want:     "diff --git a/cmd/main.go b/cmd/main.go",
		},
		{
			name:    "rejects option-like file",
			file:    "-R",
			wantErr: "must not start with '-'",
		},
		{
			name:    "rejects escaping file",
			file:    "../../etc/passwd",
			wantErr: "'..' sequences are not allowed",
		},
		{
			name:    "command failure surfaces output",
			output:  "fatal: not a git repository",
			runErr:  errors.New("exit status 128"),
			wantErr: "git diff failed: fatal: not a git repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeGit(t, nil, gitResult{tt.output, tt.runErr})

			got, err := Diff(repo, tt.staged, tt.file)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Diff() error = %v, want containing %q", err, tt.wantErr)
				}
				if tt.runErr == nil && len(*calls) != 0 {
					t.Errorf("git was executed for invalid input: %v", *calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Diff() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Diff() = %q, want %q", got, tt.want)
			}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], tt.wantArgs) {
				t.Errorf("git args = %v, want [%v]", *calls, tt.wantArgs)
			}
		})
	}
}

func TestBranches(t *testing.T) {
	repo := t.TempDir()

	tests := []struct {
		name    string
		output  string
		runErr  error
		want    []Branch
		wantErr string
	}{
		{
			name: "parses local and remote branches with current marked",
			output: "*refs/heads/main\n" +
				" refs/heads/feature/login\n" +
				" refs/remotes/origin/HEAD\n" +
				" refs/remotes/origin/main\n" +
				" refs/remotes/origin/feature/login\n",
			want: []Branch{
				{Name: "main", Current: true},
				{Name: "feature/login"},
				{Name: "origin/main", Remote: true},
				{Name: "origin/feature/login", Remote: true},
			},
		},
		{
			name:   "empty output yields empty slice",
			output: "",
			want:   []Branch{},
		},
		{
			name:    "command failure surfaces output",
			output:  "fatal: not a git repository",
			runErr:  errors.New("exit status 128"),
			wantErr: "git branch failed: fatal: not a git repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeGit(t, nil, gitResult{tt.output, tt.runErr})

			got, err := Branches(repo)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Branches() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Branches() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Branches() = %+v, want %+v", got, tt.want)
			}
			wantArgs := []string{"-C", repo, "branch", "-a", "--format=%(HEAD)%(refname)"}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], wantArgs) {
				t.Errorf("git args = %v, want [%v]", *calls, wantArgs)
			}
		})
	}
}

func TestShow(t *testing.T) {
	repo := t.TempDir()

	tests := []struct {
		name    string
		ref     string
		output  string
		runErr  error
		want    string
		wantErr string
	}{
		{
			name:   "shows HEAD",
			ref:    "HEAD",
			output: "aaa111|Alice|2026-07-01|feat: thing\n main.go | 2 +-\n",
			want:   "aaa111|Alice|2026-07-01|feat: thing\n main.go | 2 +-",
		},
		{
			name:   "shows a tag",
			ref:    "v1.2.3",
			output: "bbb222|Bob|2026-06-01|release\n",
			want:   "bbb222|Bob|2026-06-01|release",
		},
		{
			name:    "rejects option-like ref",
			ref:     "-p",
			wantErr: "invalid ref",
		},
		{
			name:    "rejects ref with dotdot",
			ref:     "main..evil",
			wantErr: "'..' sequences are not allowed",
		},
		{
			name:    "rejects empty ref",
			ref:     "",
			wantErr: "ref must not be empty",
		},
		{
			name:    "command failure surfaces output",
			ref:     "HEAD",
			output:  "fatal: bad revision 'HEAD'",
			runErr:  errors.New("exit status 128"),
			wantErr: "git show failed: fatal: bad revision 'HEAD'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeGit(t, nil, gitResult{tt.output, tt.runErr})

			got, err := Show(repo, tt.ref)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Show() error = %v, want containing %q", err, tt.wantErr)
				}
				if tt.runErr == nil && len(*calls) != 0 {
					t.Errorf("git was executed for invalid ref: %v", *calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Show() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Show() = %q, want %q", got, tt.want)
			}
			wantArgs := []string{"-C", repo, "show", "--stat", "--pretty=format:%H|%an|%ad|%s", "--date=iso", tt.ref}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], wantArgs) {
				t.Errorf("git args = %v, want [%v]", *calls, wantArgs)
			}
		})
	}
}

func TestRemotes(t *testing.T) {
	repo := t.TempDir()

	tests := []struct {
		name    string
		output  string
		runErr  error
		want    []Remote
		wantErr string
	}{
		{
			name: "dedupes fetch and push URLs sorted by name",
			output: "upstream\thttps://github.com/other/repo.git (fetch)\n" +
				"upstream\thttps://github.com/other/repo.git (push)\n" +
				"origin\thttps://github.com/user/repo.git (fetch)\n" +
				"origin\tgit@github.com:user/repo.git (push)\n",
			want: []Remote{
				{Name: "origin", FetchURL: "https://github.com/user/repo.git", PushURL: "git@github.com:user/repo.git"},
				{Name: "upstream", FetchURL: "https://github.com/other/repo.git", PushURL: "https://github.com/other/repo.git"},
			},
		},
		{
			name:   "no remotes yields empty slice",
			output: "",
			want:   []Remote{},
		},
		{
			name:    "command failure surfaces output",
			output:  "fatal: not a git repository",
			runErr:  errors.New("exit status 128"),
			wantErr: "git remote failed: fatal: not a git repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeGit(t, nil, gitResult{tt.output, tt.runErr})

			got, err := Remotes(repo)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Remotes() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Remotes() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Remotes() = %+v, want %+v", got, tt.want)
			}
			wantArgs := []string{"-C", repo, "remote", "-v"}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], wantArgs) {
				t.Errorf("git args = %v, want [%v]", *calls, wantArgs)
			}
		})
	}
}

func TestPull(t *testing.T) {
	repo := t.TempDir()

	t.Run("always fast-forward-only", func(t *testing.T) {
		calls := fakeGit(t, nil, gitResult{"Already up to date.\n", nil})

		got, err := Pull(repo)
		if err != nil {
			t.Fatalf("Pull() error = %v", err)
		}
		if got != "Already up to date." {
			t.Errorf("Pull() = %q, want %q", got, "Already up to date.")
		}
		wantArgs := []string{"-C", repo, "pull", "--ff-only"}
		if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], wantArgs) {
			t.Errorf("git args = %v, want [%v]", *calls, wantArgs)
		}
	})

	t.Run("non-fast-forward failure surfaces output", func(t *testing.T) {
		fakeGit(t, nil, gitResult{"fatal: Not possible to fast-forward, aborting.\n", errors.New("exit status 128")})

		_, err := Pull(repo)
		if err == nil || !strings.Contains(err.Error(), "git pull failed: fatal: Not possible to fast-forward") {
			t.Fatalf("Pull() error = %v, want fast-forward failure", err)
		}
	})

	t.Run("missing binary", func(t *testing.T) {
		calls := fakeGit(t, errors.New("not found"))

		if _, err := Pull(repo); err == nil || !strings.Contains(err.Error(), "git CLI not found in PATH") {
			t.Fatalf("Pull() error = %v, want missing-binary error", err)
		}
		if len(*calls) != 0 {
			t.Errorf("git was executed despite missing binary: %v", *calls)
		}
	})
}

func TestClone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clone-target")

	tests := []struct {
		name    string
		url     string
		dir     string
		output  string
		runErr  error
		want    string
		wantErr string
	}{
		{
			name:   "https URL",
			url:    "https://github.com/user/repo.git",
			dir:    dir,
			output: "Cloning into 'clone-target'...\n",
			want:   "Cloning into 'clone-target'...",
		},
		{
			name:   "ssh URL",
			url:    "ssh://git@github.com/user/repo.git",
			dir:    dir,
			output: "Cloning into 'clone-target'...\n",
			want:   "Cloning into 'clone-target'...",
		},
		{
			name:   "scp-style URL",
			url:    "git@github.com:user/repo.git",
			dir:    dir,
			output: "Cloning into 'clone-target'...\n",
			want:   "Cloning into 'clone-target'...",
		},
		{
			name:    "rejects ext transport",
			url:     "ext::sh -c whoami",
			dir:     dir,
			wantErr: "invalid clone URL",
		},
		{
			name:    "rejects fd transport",
			url:     "fd::17",
			dir:     dir,
			wantErr: "invalid clone URL",
		},
		{
			name:    "rejects file scheme",
			url:     "file:///etc/passwd",
			dir:     dir,
			wantErr: "invalid clone URL",
		},
		{
			name:    "rejects option injection",
			url:     "--upload-pack=/tmp/evil",
			dir:     dir,
			wantErr: "invalid clone URL",
		},
		{
			name:    "rejects plain http",
			url:     "http://github.com/user/repo.git",
			dir:     dir,
			wantErr: "invalid clone URL",
		},
		{
			name:    "rejects git protocol",
			url:     "git://github.com/user/repo.git",
			dir:     dir,
			wantErr: "invalid clone URL",
		},
		{
			name:    "rejects https with userinfo",
			url:     "https://user:pass@github.com/user/repo.git",
			dir:     dir,
			wantErr: "invalid clone URL",
		},
		{
			name:    "rejects local path as URL",
			url:     "/tmp/some/repo",
			dir:     dir,
			wantErr: "invalid clone URL",
		},
		{
			name:    "rejects scp-style option-like path",
			url:     "git@github.com:-repo/x",
			dir:     dir,
			wantErr: "invalid clone URL",
		},
		{
			name:    "rejects empty URL",
			url:     "",
			dir:     dir,
			wantErr: "url must not be empty",
		},
		{
			name:    "rejects relative dir",
			url:     "https://github.com/user/repo.git",
			dir:     "clone-target",
			wantErr: "must be absolute",
		},
		{
			name:    "rejects option-like dir",
			url:     "https://github.com/user/repo.git",
			dir:     "--separate-git-dir=/tmp/x",
			wantErr: "must not start with '-'",
		},
		{
			name:    "clone failure surfaces output",
			url:     "https://github.com/user/repo.git",
			dir:     dir,
			output:  "fatal: repository 'https://github.com/user/repo.git' not found",
			runErr:  errors.New("exit status 128"),
			wantErr: "git clone failed: fatal: repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeGit(t, nil, gitResult{tt.output, tt.runErr})

			got, err := Clone(tt.url, tt.dir)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Clone() error = %v, want containing %q", err, tt.wantErr)
				}
				if tt.runErr == nil && len(*calls) != 0 {
					t.Errorf("git was executed for invalid input: %v", *calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Clone() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Clone() = %q, want %q", got, tt.want)
			}
			wantArgs := []string{"clone", "--", tt.url, tt.dir}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], wantArgs) {
				t.Errorf("git args = %v, want [%v]", *calls, wantArgs)
			}
		})
	}
}

func TestCloneExpandsTildeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	calls := fakeGit(t, nil, gitResult{"done\n", nil})

	if _, err := Clone("https://github.com/user/repo.git", "~/repos/target"); err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	wantArgs := []string{"clone", "--", "https://github.com/user/repo.git", filepath.Join(home, "repos/target")}
	if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], wantArgs) {
		t.Errorf("git args = %v, want [%v]", *calls, wantArgs)
	}
}

func TestCheckout(t *testing.T) {
	repo := t.TempDir()

	tests := []struct {
		name     string
		branch   string
		create   bool
		output   string
		runErr   error
		wantArgs []string
		want     string
		wantErr  string
	}{
		{
			name:     "switches to an existing branch",
			branch:   "main",
			output:   "Switched to branch 'main'\n",
			wantArgs: []string{"-C", repo, "checkout", "main"},
			want:     "Switched to branch 'main'",
		},
		{
			name:     "create makes a new branch",
			branch:   "feature/login",
			create:   true,
			output:   "Switched to a new branch 'feature/login'\n",
			wantArgs: []string{"-C", repo, "checkout", "-b", "feature/login"},
			want:     "Switched to a new branch 'feature/login'",
		},
		{
			name:    "rejects option-like branch",
			branch:  "-b",
			wantErr: "invalid ref",
		},
		{
			name:    "rejects branch with dotdot",
			branch:  "feature/../evil",
			wantErr: "'..' sequences are not allowed",
		},
		{
			name:    "rejects empty branch",
			branch:  "",
			wantErr: "ref must not be empty",
		},
		{
			name:    "checkout failure surfaces output",
			branch:  "missing",
			output:  "error: pathspec 'missing' did not match any file(s)",
			runErr:  errors.New("exit status 1"),
			wantErr: "git checkout failed: error: pathspec 'missing'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeGit(t, nil, gitResult{tt.output, tt.runErr})

			got, err := Checkout(repo, tt.branch, tt.create)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Checkout() error = %v, want containing %q", err, tt.wantErr)
				}
				if tt.runErr == nil && len(*calls) != 0 {
					t.Errorf("git was executed for invalid branch: %v", *calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Checkout() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Checkout() = %q, want %q", got, tt.want)
			}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], tt.wantArgs) {
				t.Errorf("git args = %v, want [%v]", *calls, tt.wantArgs)
			}
		})
	}
}

func TestCommitAll(t *testing.T) {
	repo := t.TempDir()

	t.Run("stages, commits, and returns the new hash", func(t *testing.T) {
		calls := fakeGit(t, nil,
			gitResult{"", nil},
			gitResult{"[main abc1234] feat: thing\n", nil},
			gitResult{"abc1234def5678\n", nil},
		)

		hash, err := CommitAll(repo, "feat: thing")
		if err != nil {
			t.Fatalf("CommitAll() error = %v", err)
		}
		if hash != "abc1234def5678" {
			t.Errorf("CommitAll() = %q, want %q", hash, "abc1234def5678")
		}
		wantCalls := [][]string{
			{"-C", repo, "add", "-A"},
			{"-C", repo, "commit", "-m", "feat: thing"},
			{"-C", repo, "rev-parse", "HEAD"},
		}
		if !reflect.DeepEqual(*calls, wantCalls) {
			t.Errorf("git calls = %v, want %v", *calls, wantCalls)
		}
	})

	t.Run("rejects empty message", func(t *testing.T) {
		calls := fakeGit(t, nil)

		if _, err := CommitAll(repo, ""); err == nil || !strings.Contains(err.Error(), "commit message must not be empty") {
			t.Fatalf("CommitAll() error = %v, want empty-message error", err)
		}
		if len(*calls) != 0 {
			t.Errorf("git was executed for empty message: %v", *calls)
		}
	})

	t.Run("rejects whitespace-only message", func(t *testing.T) {
		calls := fakeGit(t, nil)

		if _, err := CommitAll(repo, "  \n\t"); err == nil || !strings.Contains(err.Error(), "commit message must not be empty") {
			t.Fatalf("CommitAll() error = %v, want empty-message error", err)
		}
		if len(*calls) != 0 {
			t.Errorf("git was executed for whitespace message: %v", *calls)
		}
	})

	t.Run("add failure stops before commit", func(t *testing.T) {
		calls := fakeGit(t, nil, gitResult{"fatal: not a git repository\n", errors.New("exit status 128")})

		if _, err := CommitAll(repo, "feat: thing"); err == nil || !strings.Contains(err.Error(), "git add failed: fatal: not a git repository") {
			t.Fatalf("CommitAll() error = %v, want add failure", err)
		}
		if len(*calls) != 1 {
			t.Errorf("git calls = %v, want only the add invocation", *calls)
		}
	})

	t.Run("commit failure surfaces output", func(t *testing.T) {
		calls := fakeGit(t, nil,
			gitResult{"", nil},
			gitResult{"nothing to commit, working tree clean\n", errors.New("exit status 1")},
		)

		if _, err := CommitAll(repo, "feat: thing"); err == nil || !strings.Contains(err.Error(), "git commit failed: nothing to commit") {
			t.Fatalf("CommitAll() error = %v, want commit failure", err)
		}
		if len(*calls) != 2 {
			t.Errorf("git calls = %v, want add and commit only", *calls)
		}
	})
}

func TestPush(t *testing.T) {
	repo := t.TempDir()

	tests := []struct {
		name     string
		remote   string
		branch   string
		output   string
		runErr   error
		wantArgs []string
		want     string
		wantErr  string
	}{
		{
			name:     "default pushes current branch to upstream",
			output:   "Everything up-to-date\n",
			wantArgs: []string{"-C", repo, "push"},
			want:     "Everything up-to-date",
		},
		{
			name:     "explicit remote",
			remote:   "origin",
			output:   "done\n",
			wantArgs: []string{"-C", repo, "push", "origin"},
			want:     "done",
		},
		{
			name:     "explicit remote and branch",
			remote:   "origin",
			branch:   "main",
			output:   "done\n",
			wantArgs: []string{"-C", repo, "push", "origin", "main"},
			want:     "done",
		},
		{
			name:    "branch without remote is rejected",
			branch:  "main",
			wantErr: "remote is required when branch is set",
		},
		{
			name:    "rejects force flag smuggled as remote",
			remote:  "--force",
			wantErr: "invalid ref",
		},
		{
			name:    "rejects force flag smuggled as branch",
			remote:  "origin",
			branch:  "-f",
			wantErr: "invalid ref",
		},
		{
			name:    "rejects branch with dotdot",
			remote:  "origin",
			branch:  "a..b",
			wantErr: "'..' sequences are not allowed",
		},
		{
			name:    "push failure surfaces output",
			output:  "! [rejected] main -> main (non-fast-forward)\n",
			runErr:  errors.New("exit status 1"),
			wantErr: "git push failed: ! [rejected] main -> main (non-fast-forward)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeGit(t, nil, gitResult{tt.output, tt.runErr})

			got, err := Push(repo, tt.remote, tt.branch)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Push() error = %v, want containing %q", err, tt.wantErr)
				}
				if tt.runErr == nil && len(*calls) != 0 {
					t.Errorf("git was executed for invalid input: %v", *calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Push() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Push() = %q, want %q", got, tt.want)
			}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], tt.wantArgs) {
				t.Errorf("git args = %v, want [%v]", *calls, tt.wantArgs)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr string
	}{
		{"absolute path", "/srv/repos/x", "/srv/repos/x", ""},
		{"absolute path is cleaned", "/srv/repos/../repos/x", "/srv/repos/x", ""},
		{"bare tilde", "~", home, ""},
		{"tilde subpath", "~/repos/x", filepath.Join(home, "repos/x"), ""},
		{"tilde other user unsupported", "~other/x", "", "must be absolute"},
		{"relative path", "repos/x", "", "must be absolute"},
		{"dot relative path", "./x", "", "must be absolute"},
		{"option-like path", "-C/evil", "", "must not start with '-'"},
		{"empty path", "", "", "path must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidatePath(tt.path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ValidatePath(%q) error = %v, want containing %q", tt.path, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidatePath(%q) error = %v", tt.path, err)
			}
			if got != tt.want {
				t.Errorf("ValidatePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestValidateRef(t *testing.T) {
	tests := []struct {
		ref     string
		wantErr bool
	}{
		{"main", false},
		{"feature/login", false},
		{"v1.2.3", false},
		{"HEAD", false},
		{"origin", false},
		{"release-2026.07", false},
		{"abc1234def", false},
		{"", true},
		{"-b", true},
		{"--force", true},
		{"main..evil", true},
		{"feature/../evil", true},
		{".hidden", true},
		{"/leading-slash", true},
		{"branch name", true},
		{"branch;reboot", true},
		{"$(whoami)", true},
		{"`id`", true},
		{strings.Repeat("a", 256), true},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			err := ValidateRef(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRef(%q) error = %v, wantErr %v", tt.ref, err, tt.wantErr)
			}
		})
	}
}

func TestValidateCloneURL(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://github.com/user/repo.git", false},
		{"https://github.com/user/repo", false},
		{"https://git.example.com:8443/team/repo.git", false},
		{"ssh://git@github.com/user/repo.git", false},
		{"ssh://github.com/repo", false},
		{"ssh://git@git.example.com:2222/team/repo.git", false},
		{"git@github.com:user/repo.git", false},
		{"git@gitlab.com:group/sub/repo", false},
		{"git@server.local:/srv/git/repo.git", false},
		{"", true},
		{"ext::sh -c id", true},
		{"fd::17", true},
		{"file:///etc/passwd", true},
		{"http://github.com/user/repo.git", true},
		{"git://github.com/user/repo.git", true},
		{"--upload-pack=/tmp/evil", true},
		{"-o=x", true},
		{"https://user:pass@github.com/user/repo.git", true},
		{"https://github.com/user/repo.git;id", true},
		{"git@github.com:-repo/x", true},
		{"git@github.com:user/repo.git extra", true},
		{"/local/path/repo", true},
		{"ssh://-host/repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := ValidateCloneURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCloneURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

// TestTimeoutsByOperation verifies that local operations run under the short
// timeout and network operations (pull, clone, push) get the longer one.
func TestTimeoutsByOperation(t *testing.T) {
	repo := t.TempDir()

	tests := []struct {
		name    string
		call    func() error
		network bool
	}{
		{"status is local", func() error { _, err := Status(repo); return err }, false},
		{"checkout is local", func() error { _, err := Checkout(repo, "main", false); return err }, false},
		{"pull is network", func() error { _, err := Pull(repo); return err }, true},
		{"clone is network", func() error {
			_, err := Clone("https://github.com/user/repo.git", filepath.Join(repo, "dst"))
			return err
		}, true},
		{"push is network", func() error { _, err := Push(repo, "", ""); return err }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origLook, origRun := lookGit, runGit
			t.Cleanup(func() {
				lookGit, runGit = origLook, origRun
			})

			var remaining time.Duration
			lookGit = func() error { return nil }
			runGit = func(ctx context.Context, args ...string) ([]byte, error) {
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("git invoked without a context deadline")
				}
				remaining = time.Until(deadline)
				return []byte("## main\n"), nil
			}

			if err := tt.call(); err != nil {
				t.Fatalf("call failed: %v", err)
			}
			if tt.network && remaining <= localTimeout {
				t.Errorf("deadline %v away, want more than %v for a network operation", remaining, localTimeout)
			}
			if !tt.network && remaining > localTimeout {
				t.Errorf("deadline %v away, want at most %v for a local operation", remaining, localTimeout)
			}
		})
	}
}

// TestNewGitCmdDisablesPrompts verifies (without executing anything) that
// every real git invocation carries the environment that disables interactive
// credential prompts, so git fails fast instead of hanging.
func TestNewGitCmdDisablesPrompts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cmd := newGitCmd(ctx, "status")

	for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=true"} {
		found := false
		for _, e := range cmd.Env {
			if e == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("git command env missing %s", want)
		}
	}
	wantArgs := []string{"git", "status"}
	if !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Errorf("cmd.Args = %v, want %v", cmd.Args, wantArgs)
	}
}
