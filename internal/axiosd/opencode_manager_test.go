package axiosd

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/axios-os/axios/pkg/opencode"
	"github.com/axios-os/axios/pkg/permissions"
)

// fakeOpencodeClient implements opencodeAPI for tests.
type fakeOpencodeClient struct {
	mu           sync.Mutex
	permReplies  []string // "sessionID/permID/response"
	prompts      []string
	promptModels []string // "provider/model" per prompt, "" when nil
	aborted      []string
	sessionSeq   int
	messages     map[string][]opencode.Message
	diffs        map[string][]opencode.FileDiff
	createErr    error
	promptErr    error
}

func (f *fakeOpencodeClient) Health() error { return nil }

func (f *fakeOpencodeClient) CreateSession(dir, title string) (*opencode.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.sessionSeq++
	return &opencode.Session{ID: fmt.Sprintf("ses_%d", f.sessionSeq), Directory: dir, Title: title}, nil
}

func (f *fakeOpencodeClient) PromptAsync(sessionID string, model *opencode.ModelRef, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.promptErr != nil {
		return f.promptErr
	}
	f.prompts = append(f.prompts, sessionID+": "+text)
	modelRef := ""
	if model != nil {
		modelRef = model.ProviderID + "/" + model.ModelID
	}
	f.promptModels = append(f.promptModels, modelRef)
	return nil
}

func (f *fakeOpencodeClient) Messages(sessionID string) ([]opencode.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.messages[sessionID], nil
}

func (f *fakeOpencodeClient) ReplyPermission(sessionID, permID, response string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.permReplies = append(f.permReplies, sessionID+"/"+permID+"/"+response)
	return nil
}

func (f *fakeOpencodeClient) Abort(sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aborted = append(f.aborted, sessionID)
	return nil
}

func (f *fakeOpencodeClient) Diff(sessionID string) ([]opencode.FileDiff, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.diffs[sessionID], nil
}

func (f *fakeOpencodeClient) Events(ctx context.Context) (<-chan opencode.Event, error) {
	ch := make(chan opencode.Event)
	go func() { <-ctx.Done(); close(ch) }()
	return ch, nil
}

func (f *fakeOpencodeClient) replies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.permReplies...)
}

// newTestOpencodeManager builds a live-looking manager around a fake client
// without spawning any process.
func newTestOpencodeManager(t *testing.T, client opencodeAPI) *OpencodeManager {
	t.Helper()
	m := NewOpencodeManager(OpencodeOptions{Enabled: true, Workspace: t.TempDir()}, nil, "", testLogger())
	m.client = client
	m.ctx = context.Background()
	m.ready = true
	return m
}

func TestOpencodeServeArgs(t *testing.T) {
	args := opencodeServeArgs(4097)
	want := []string{"serve", "--port", "4097", "--hostname", "127.0.0.1", "--print-logs", "--log-level", "INFO"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestBuildOpencodeEnv(t *testing.T) {
	env := buildOpencodeEnv([]string{"HOME=/tmp/h"}, "s3cret", map[string]string{"ANTHROPIC_API_KEY": "sk-test"})

	find := func(key string) (string, bool) {
		for _, kv := range env {
			if v, ok := strings.CutPrefix(kv, key+"="); ok {
				return v, true
			}
		}
		return "", false
	}

	if v, ok := find("OPENCODE_SERVER_PASSWORD"); !ok || v != "s3cret" {
		t.Errorf("OPENCODE_SERVER_PASSWORD = %q, %v", v, ok)
	}
	if v, ok := find("ANTHROPIC_API_KEY"); !ok || v != "sk-test" {
		t.Errorf("ANTHROPIC_API_KEY = %q, %v", v, ok)
	}
	if _, ok := find("HOME"); !ok {
		t.Error("base environment was dropped")
	}

	cfgJSON, ok := find("OPENCODE_CONFIG_CONTENT")
	if !ok {
		t.Fatal("OPENCODE_CONFIG_CONTENT missing")
	}
	var cfg struct {
		Permission map[string]json.RawMessage `json:"permission"`
	}
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		t.Fatalf("OPENCODE_CONFIG_CONTENT is not valid JSON: %v", err)
	}
	var bash map[string]string
	if err := json.Unmarshal(cfg.Permission["bash"], &bash); err != nil {
		t.Fatalf("permission.bash is not a rule map: %v", err)
	}
	for pattern, action := range map[string]string{"rm -rf *": "deny", "sudo *": "deny", "*": "ask"} {
		if bash[pattern] != action {
			t.Errorf("permission.bash[%q] = %q, want %q", pattern, bash[pattern], action)
		}
	}
	var wildcard string
	if err := json.Unmarshal(cfg.Permission["*"], &wildcard); err != nil || wildcard != "ask" {
		t.Errorf("permission[*] = %q (err %v), want \"ask\"", wildcard, err)
	}
}

func TestOpencodeCredentialEnv(t *testing.T) {
	store := NewProviderStore(filepath.Join(t.TempDir(), "providers.json"), nil)
	if err := store.SetAPIKey("anthropic", "sk-ant-test"); err != nil {
		t.Fatalf("SetAPIKey: %v", err)
	}
	creds := opencodeCredentialEnv(store)
	if creds["ANTHROPIC_API_KEY"] != "sk-ant-test" {
		t.Errorf("creds = %v, want ANTHROPIC_API_KEY=sk-ant-test", creds)
	}
	if len(creds) != 1 {
		t.Errorf("unexpected extra credentials: %v", creds)
	}
	if got := opencodeCredentialEnv(nil); len(got) != 0 {
		t.Errorf("nil store should yield no credentials, got %v", got)
	}
}

func TestHandlePermissionAskedDecisionTable(t *testing.T) {
	tests := []struct {
		name     string
		tier     permissions.Tier
		approve  bool
		want     string // expected ReplyPermission response
		wantTool string // tool name passed to the checker
	}{
		{"trusted allows once", permissions.Trusted, false, opencode.PermissionOnce, "opencode__bash"},
		{"prohibited rejects", permissions.Prohibited, true, opencode.PermissionReject, "opencode__bash"},
		{"approval approved allows once", permissions.ApprovalRequired, true, opencode.PermissionOnce, "opencode__bash"},
		{"approval denied rejects", permissions.ApprovalRequired, false, opencode.PermissionReject, "opencode__bash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeOpencodeClient{}
			m := newTestOpencodeManager(t, client)

			var checkedTool string
			m.bind(
				func(toolName string, args map[string]any) permissions.Tier {
					checkedTool = toolName
					return tt.tier
				},
				func(ctx context.Context, tool string, params json.RawMessage) bool {
					return tt.approve
				},
			)

			m.handlePermissionAsked(opencode.PermissionAsked{
				ID:         "per_1",
				SessionID:  "ses_1",
				Permission: "bash",
				Metadata:   json.RawMessage(`{"command":"go test ./..."}`),
			})

			replies := client.replies()
			if len(replies) != 1 {
				t.Fatalf("got %d permission replies, want 1: %v", len(replies), replies)
			}
			if want := "ses_1/per_1/" + tt.want; replies[0] != want {
				t.Errorf("reply = %q, want %q", replies[0], want)
			}
			if checkedTool != tt.wantTool {
				t.Errorf("checker saw tool %q, want %q", checkedTool, tt.wantTool)
			}
		})
	}
}

func TestDelegateAndFinalize(t *testing.T) {
	client := &fakeOpencodeClient{
		messages: map[string][]opencode.Message{
			"ses_1": {
				{
					Info: opencode.MessageInfo{Role: "assistant", Cost: 0.25,
						Tokens: opencode.TokenUsage{Input: 100, Output: 50}},
					Parts: []opencode.Part{{Type: "text", Text: "All tests pass."}},
				},
			},
		},
	}
	m := newTestOpencodeManager(t, client)

	task, err := m.Delegate("fix the flaky test", "", nil)
	if err != nil {
		t.Fatalf("Delegate: %v", err)
	}
	if task.Status != TaskRunning || task.SessionID != "ses_1" {
		t.Fatalf("task = %+v, want running on ses_1", task)
	}

	m.finalizeTask("ses_1", "")

	done, ok := m.Task(task.ID)
	if !ok {
		t.Fatal("task disappeared")
	}
	if done.Status != TaskDone {
		t.Errorf("status = %s, want done", done.Status)
	}
	if done.Result != "All tests pass." {
		t.Errorf("result = %q", done.Result)
	}
	if done.CostUSD != 0.25 || done.InputTokens != 100 || done.OutputTokens != 50 {
		t.Errorf("accounting = %v/%v/%v, want 0.25/100/50", done.CostUSD, done.InputTokens, done.OutputTokens)
	}

	// finalize is idempotent on terminal tasks.
	m.finalizeTask("ses_1", "boom")
	again, _ := m.Task(task.ID)
	if again.Status != TaskDone {
		t.Errorf("terminal task was re-finalized to %s", again.Status)
	}
}

func TestDelegateFailures(t *testing.T) {
	t.Run("empty prompt", func(t *testing.T) {
		m := newTestOpencodeManager(t, &fakeOpencodeClient{})
		if _, err := m.Delegate("  ", "", nil); err == nil {
			t.Error("empty prompt should fail")
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		m := newTestOpencodeManager(t, &fakeOpencodeClient{})
		if _, err := m.Delegate("do it", "/definitely/not/a/dir", nil); err == nil {
			t.Error("nonexistent directory should fail")
		}
	})

	t.Run("prompt submit failure marks task failed", func(t *testing.T) {
		client := &fakeOpencodeClient{promptErr: fmt.Errorf("boom")}
		m := newTestOpencodeManager(t, client)
		if _, err := m.Delegate("do it", "", nil); err == nil {
			t.Fatal("PromptAsync failure should surface")
		}
		tasks := m.Tasks()
		if len(tasks) != 1 || tasks[0].Status != TaskFailed {
			t.Errorf("tasks = %+v, want one failed task", tasks)
		}
	})

	t.Run("not ready", func(t *testing.T) {
		m := newTestOpencodeManager(t, &fakeOpencodeClient{})
		m.ready = false
		if _, err := m.Delegate("do it", "", nil); err == nil {
			t.Error("unready manager should refuse work")
		}
	})
}

func TestOpencodeTaskStorePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	logger := testLogger()

	store := newOpencodeTaskStore(path, logger)
	now := time.Now()
	completed := now.Add(time.Minute)
	store.add(&OpencodeTask{ID: "oct-1", SessionID: "ses_1", Prompt: "a", Status: TaskRunning, CreatedAt: now})
	store.add(&OpencodeTask{ID: "oct-2", SessionID: "ses_2", Prompt: "b", Status: TaskDone, CreatedAt: now, CompletedAt: &completed, Result: "ok"})

	// Reload: the finished task survives verbatim; the in-flight one is
	// marked failed (its opencode server did not outlive the daemon).
	reloaded := newOpencodeTaskStore(path, logger)
	t1, ok := reloaded.get("oct-1")
	if !ok || t1.Status != TaskFailed || t1.Error == "" {
		t.Errorf("in-flight task after reload = %+v, want failed with error", t1)
	}
	t2, ok := reloaded.get("oct-2")
	if !ok || t2.Status != TaskDone || t2.Result != "ok" {
		t.Errorf("finished task after reload = %+v, want done/ok", t2)
	}
	if _, ok := reloaded.byOpencodeSession("ses_2"); !ok {
		t.Error("session index not rebuilt on load")
	}
}

func TestExecuteChatTool(t *testing.T) {
	client := &fakeOpencodeClient{}
	m := newTestOpencodeManager(t, client)

	out := m.ExecuteChatTool("delegate_task", map[string]any{"prompt": "add tests"})
	if !strings.Contains(out, "delegated") || !strings.Contains(out, "oct-") {
		t.Errorf("delegate_task output = %q", out)
	}

	tasks := m.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("want one task, got %d", len(tasks))
	}
	status := m.ExecuteChatTool("task_status", map[string]any{"task_id": tasks[0].ID})
	if !strings.Contains(status, tasks[0].ID) || !strings.Contains(status, string(TaskRunning)) {
		t.Errorf("task_status output = %q", status)
	}

	if out := m.ExecuteChatTool("task_status", map[string]any{"task_id": "nope"}); !strings.Contains(out, "error") {
		t.Errorf("unknown task should error, got %q", out)
	}
	if out := m.ExecuteChatTool("bogus", nil); !strings.Contains(out, "unknown opencode tool") {
		t.Errorf("unknown tool should error, got %q", out)
	}
}
