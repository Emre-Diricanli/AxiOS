package axiosd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"
)

// OpencodeTaskStatus is the lifecycle state of a delegated coding task.
type OpencodeTaskStatus string

const (
	TaskQueued  OpencodeTaskStatus = "queued"
	TaskRunning OpencodeTaskStatus = "running"
	TaskDone    OpencodeTaskStatus = "done"
	TaskFailed  OpencodeTaskStatus = "failed"
	TaskAborted OpencodeTaskStatus = "aborted"
)

// terminal reports whether the status is final (no further transitions).
func (s OpencodeTaskStatus) terminal() bool {
	switch s {
	case TaskDone, TaskFailed, TaskAborted:
		return true
	}
	return false
}

// OpencodeTask is one coding task delegated to the managed opencode server.
// Cost and token counts are summed over the session's assistant messages
// (opencode MessageInfo); Result is the text of the last assistant message.
type OpencodeTask struct {
	ID           string             `json:"id"`
	SessionID    string             `json:"session_id"` // opencode session id
	Prompt       string             `json:"prompt"`
	Directory    string             `json:"directory"`
	Status       OpencodeTaskStatus `json:"status"`
	CreatedAt    time.Time          `json:"created_at"`
	CompletedAt  *time.Time         `json:"completed_at,omitempty"`
	Result       string             `json:"result,omitempty"`
	CostUSD      float64            `json:"cost_usd,omitempty"`
	InputTokens  int                `json:"input_tokens,omitempty"`
	OutputTokens int                `json:"output_tokens,omitempty"`
	Error        string             `json:"last_error,omitempty"`
}

// opencodeTasksFileVersion is the on-disk format version of opencode_tasks.json.
const opencodeTasksFileVersion = 1

// opencodeTasksFile is the persisted structure at $AXIOS_DATA_DIR/opencode_tasks.json.
type opencodeTasksFile struct {
	Version int             `json:"version"`
	Tasks   []*OpencodeTask `json:"tasks"`
}

// opencodeTaskStore tracks delegated tasks: an in-memory map persisted to a
// JSON file on every mutation. Accessors return copies so callers never share
// mutable state with the store.
type opencodeTaskStore struct {
	mu        sync.Mutex
	tasks     map[string]*OpencodeTask // task id -> task
	bySession map[string]string        // opencode session id -> task id
	filePath  string                   // "" = in-memory only (tests)
	logger    *slog.Logger
}

// newOpencodeTaskStore creates a store persisting to filePath (loading any
// existing file). Tasks that were still queued/running when the daemon last
// exited are marked failed: the opencode server that ran them is gone.
func newOpencodeTaskStore(filePath string, logger *slog.Logger) *opencodeTaskStore {
	if logger == nil {
		logger = slog.Default()
	}
	ts := &opencodeTaskStore{
		tasks:     make(map[string]*OpencodeTask),
		bySession: make(map[string]string),
		filePath:  filePath,
		logger:    logger,
	}
	ts.load()
	return ts
}

// add inserts a task and persists.
func (ts *opencodeTaskStore) add(t *OpencodeTask) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.tasks[t.ID] = t
	if t.SessionID != "" {
		ts.bySession[t.SessionID] = t.ID
	}
	ts.save()
}

// get returns a copy of the task with the given id.
func (ts *opencodeTaskStore) get(id string) (OpencodeTask, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	t, ok := ts.tasks[id]
	if !ok {
		return OpencodeTask{}, false
	}
	return *t, true
}

// byOpencodeSession returns a copy of the task owning an opencode session id.
func (ts *opencodeTaskStore) byOpencodeSession(sessionID string) (OpencodeTask, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	id, ok := ts.bySession[sessionID]
	if !ok {
		return OpencodeTask{}, false
	}
	t, ok := ts.tasks[id]
	if !ok {
		return OpencodeTask{}, false
	}
	return *t, true
}

// update applies fn to the stored task under the lock and persists. It
// reports whether the task existed.
func (ts *opencodeTaskStore) update(id string, fn func(*OpencodeTask)) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	t, ok := ts.tasks[id]
	if !ok {
		return false
	}
	fn(t)
	ts.save()
	return true
}

// list returns copies of all tasks, most recently created first.
func (ts *opencodeTaskStore) list() []OpencodeTask {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	out := make([]OpencodeTask, 0, len(ts.tasks))
	for _, t := range ts.tasks {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// save persists the store (caller must hold ts.mu). Errors are logged, not
// returned — task tracking must never take the daemon down.
func (ts *opencodeTaskStore) save() {
	if ts.filePath == "" {
		return
	}
	file := opencodeTasksFile{Version: opencodeTasksFileVersion}
	for _, t := range ts.tasks {
		file.Tasks = append(file.Tasks, t)
	}
	sort.Slice(file.Tasks, func(i, j int) bool {
		if !file.Tasks[i].CreatedAt.Equal(file.Tasks[j].CreatedAt) {
			return file.Tasks[i].CreatedAt.Before(file.Tasks[j].CreatedAt)
		}
		return file.Tasks[i].ID < file.Tasks[j].ID
	})
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		ts.logger.Error("failed to marshal opencode tasks", "error", err)
		return
	}
	// 0600: prompts and results may contain sensitive code and command output.
	if err := os.WriteFile(ts.filePath, data, 0o600); err != nil {
		ts.logger.Error("failed to save opencode tasks", "path", ts.filePath, "error", err)
	}
}

// load reads the persisted file, marking any task that was still in flight as
// failed (its opencode server did not survive the daemon restart).
func (ts *opencodeTaskStore) load() {
	if ts.filePath == "" {
		return
	}
	data, err := os.ReadFile(ts.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			ts.logger.Error("failed to read opencode tasks", "path", ts.filePath, "error", err)
		}
		return
	}

	var file opencodeTasksFile
	if err := json.Unmarshal(data, &file); err != nil {
		ts.logger.Error("failed to parse opencode tasks", "path", ts.filePath, "error", err)
		return
	}
	if file.Version > opencodeTasksFileVersion {
		ts.logger.Warn("opencode tasks file has a newer version — loading best-effort",
			"path", ts.filePath, "version", file.Version)
	}

	stale := 0
	now := time.Now()
	for _, t := range file.Tasks {
		if t == nil || t.ID == "" {
			continue
		}
		if !t.Status.terminal() {
			t.Status = TaskFailed
			t.Error = "interrupted by daemon restart"
			completed := now
			t.CompletedAt = &completed
			stale++
		}
		ts.tasks[t.ID] = t
		if t.SessionID != "" {
			ts.bySession[t.SessionID] = t.ID
		}
	}
	if stale > 0 {
		ts.logger.Warn("marked in-flight opencode tasks as failed after restart", "count", stale)
		ts.mu.Lock()
		ts.save()
		ts.mu.Unlock()
	}
}

// String implements fmt.Stringer for concise task logging.
func (t OpencodeTask) String() string {
	return fmt.Sprintf("task %s (%s, session %s)", t.ID, t.Status, t.SessionID)
}
