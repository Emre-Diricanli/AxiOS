package axiosd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axios-os/axios/pkg/opencode"
)

// newCodeAPITestServer wires a Server around a test opencode manager.
func newCodeAPITestServer(t *testing.T, client opencodeAPI) (*Server, *OpencodeManager) {
	t.Helper()
	m := newTestOpencodeManager(t, client)
	s := &Server{logger: testLogger()}
	s.SetOpencodeManager(m)
	return s, m
}

func TestCodeTasksDisabled(t *testing.T) {
	t.Run("nil manager", func(t *testing.T) {
		s := &Server{logger: testLogger()}
		rec := httptest.NewRecorder()
		s.handleCodeTasks(rec, httptest.NewRequest(http.MethodGet, "/api/code/tasks", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("disabled manager", func(t *testing.T) {
		s, m := newCodeAPITestServer(t, &fakeOpencodeClient{})
		m.setDisabled()
		rec := httptest.NewRecorder()
		s.handleCodeTasks(rec, httptest.NewRequest(http.MethodGet, "/api/code/tasks", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", rec.Code)
		}
	})
}

func TestCodeTasksCreateAndList(t *testing.T) {
	s, m := newCodeAPITestServer(t, &fakeOpencodeClient{})

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"prompt":"refactor the config loader"}`)
	s.handleCodeTasks(rec, httptest.NewRequest(http.MethodPost, "/api/code/tasks", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body %s", rec.Code, rec.Body)
	}
	var created struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.TaskID == "" || created.Status != string(TaskRunning) {
		t.Errorf("create response = %+v", created)
	}

	rec = httptest.NewRecorder()
	s.handleCodeTasks(rec, httptest.NewRequest(http.MethodGet, "/api/code/tasks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	var listed struct {
		Tasks []OpencodeTask `json:"tasks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.Tasks) != 1 || listed.Tasks[0].ID != created.TaskID {
		t.Errorf("list = %+v, want the created task", listed.Tasks)
	}

	// Validation errors.
	rec = httptest.NewRecorder()
	s.handleCodeTasks(rec, httptest.NewRequest(http.MethodPost, "/api/code/tasks", strings.NewReader(`{"prompt":""}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty prompt status = %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleCodeTasks(rec, httptest.NewRequest(http.MethodPost, "/api/code/tasks", strings.NewReader(`not json`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad json status = %d, want 400", rec.Code)
	}

	_ = m
}

func TestCodeTaskByID(t *testing.T) {
	client := &fakeOpencodeClient{
		diffs: map[string][]opencode.FileDiff{
			"ses_1": {{File: "main.go", Additions: 3, Deletions: 1, Status: "modified"}},
		},
	}
	s, m := newCodeAPITestServer(t, client)

	task, err := m.Delegate("do the thing", "", nil)
	if err != nil {
		t.Fatalf("Delegate: %v", err)
	}

	// GET while running: no diff yet.
	rec := httptest.NewRecorder()
	s.handleCodeTaskByID(rec, httptest.NewRequest(http.MethodGet, "/api/code/tasks/"+task.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}
	var detail codeTaskDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.ID != task.ID || len(detail.Diff) != 0 {
		t.Errorf("running detail = %+v, want no diff", detail)
	}

	// GET when done: diff included.
	m.finalizeTask("ses_1", "")
	rec = httptest.NewRecorder()
	s.handleCodeTaskByID(rec, httptest.NewRequest(http.MethodGet, "/api/code/tasks/"+task.ID, nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Status != TaskDone || len(detail.Diff) != 1 || detail.Diff[0].File != "main.go" {
		t.Errorf("done detail = %+v, want done with one diff entry", detail)
	}

	// Unknown id.
	rec = httptest.NewRecorder()
	s.handleCodeTaskByID(rec, httptest.NewRequest(http.MethodGet, "/api/code/tasks/oct-nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown id status = %d, want 404", rec.Code)
	}
}

func TestCodeTaskAbort(t *testing.T) {
	client := &fakeOpencodeClient{}
	s, m := newCodeAPITestServer(t, client)

	task, err := m.Delegate("long running", "", nil)
	if err != nil {
		t.Fatalf("Delegate: %v", err)
	}

	rec := httptest.NewRecorder()
	s.handleCodeTaskByID(rec, httptest.NewRequest(http.MethodDelete, "/api/code/tasks/"+task.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("abort status = %d, body %s", rec.Code, rec.Body)
	}
	aborted, _ := m.Task(task.ID)
	if aborted.Status != TaskAborted {
		t.Errorf("status = %s, want aborted", aborted.Status)
	}
	if len(client.aborted) != 1 || client.aborted[0] != task.SessionID {
		t.Errorf("client.Abort calls = %v", client.aborted)
	}

	// Aborting a terminal task conflicts.
	rec = httptest.NewRecorder()
	s.handleCodeTaskByID(rec, httptest.NewRequest(http.MethodDelete, "/api/code/tasks/"+task.ID, nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("double abort status = %d, want 409", rec.Code)
	}
}

func TestParseModelRef(t *testing.T) {
	if ref := parseModelRef("anthropic/claude-sonnet-4-5"); ref == nil ||
		ref.ProviderID != "anthropic" || ref.ModelID != "claude-sonnet-4-5" {
		t.Errorf("parseModelRef = %+v", ref)
	}
	for _, bad := range []string{"", "noslash", "/model", "provider/"} {
		if ref := parseModelRef(bad); ref != nil {
			t.Errorf("parseModelRef(%q) = %+v, want nil", bad, ref)
		}
	}
}

// TestDispatchToolRoutesOpencode verifies executeTool's dispatch sends the
// opencode pseudo-server to the manager, not the MCP path.
func TestDispatchToolRoutesOpencode(t *testing.T) {
	s, _ := newCodeAPITestServer(t, &fakeOpencodeClient{})
	out := s.dispatchTool(opencodeServerName, "delegate_task", map[string]any{"prompt": "hi"})
	if !strings.Contains(out, "delegated") {
		t.Errorf("dispatch output = %q, want a delegated task", out)
	}

	s2 := &Server{logger: testLogger()}
	if out := s2.dispatchTool(opencodeServerName, "delegate_task", nil); !strings.Contains(out, "not enabled") {
		t.Errorf("nil manager dispatch = %q, want not-enabled error", out)
	}
}

func TestCodeModelsAndDefaultModel(t *testing.T) {
	s, m := newCodeAPITestServer(t, &fakeOpencodeClient{})

	// GET /api/code/models lists provider/model ids with the current default.
	rec := httptest.NewRecorder()
	s.handleCodeModels(rec, httptest.NewRequest(http.MethodGet, "/api/code/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("models status = %d, body %s", rec.Code, rec.Body)
	}
	var listed struct {
		Models  []string `json:"models"`
		Default string   `json:"default"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"xai/grok-4.5": false, "xai/grok-4.3": false, "opencode/zen-free-model": false}
	for _, id := range listed.Models {
		if _, ok := want[id]; ok {
			want[id] = true
		}
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("model %q missing from %v", id, listed.Models)
		}
	}
	if listed.Default != "" {
		t.Errorf("default = %q, want empty before override", listed.Default)
	}

	// PUT /api/code/model sets the delegated-task default.
	rec = httptest.NewRecorder()
	s.handleCodeModel(rec, httptest.NewRequest(http.MethodPut, "/api/code/model",
		strings.NewReader(`{"model":"xai/grok-4.5"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("set model status = %d, body %s", rec.Code, rec.Body)
	}
	if got := m.DefaultModel(); got != "xai/grok-4.5" {
		t.Errorf("DefaultModel = %q", got)
	}

	// Delegated tasks now use it.
	if _, err := m.Delegate("task", "", nil); err != nil {
		t.Fatal(err)
	}
	client := m.client.(*fakeOpencodeClient)
	client.mu.Lock()
	lastModel := client.promptModels[len(client.promptModels)-1]
	client.mu.Unlock()
	if lastModel != "xai/grok-4.5" {
		t.Errorf("delegated prompt model = %q", lastModel)
	}

	// Malformed model rejected; clearing works.
	rec = httptest.NewRecorder()
	s.handleCodeModel(rec, httptest.NewRequest(http.MethodPut, "/api/code/model",
		strings.NewReader(`{"model":"noslash"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed model status = %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleCodeModel(rec, httptest.NewRequest(http.MethodPut, "/api/code/model",
		strings.NewReader(`{"model":""}`)))
	if rec.Code != http.StatusOK || m.DefaultModel() != "" {
		t.Errorf("clearing override failed: status %d, default %q", rec.Code, m.DefaultModel())
	}
}

func TestDefaultModelPersistsAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	tasksPath := filepath.Join(dir, "opencode_tasks.json")

	m1 := NewOpencodeManager(OpencodeOptions{Enabled: true}, nil, tasksPath, testLogger())
	if err := m1.SetDefaultModel("xai/grok-4.5"); err != nil {
		t.Fatal(err)
	}

	m2 := NewOpencodeManager(OpencodeOptions{Enabled: true, Model: "config/fallback"}, nil, tasksPath, testLogger())
	if got := m2.DefaultModel(); got != "xai/grok-4.5" {
		t.Errorf("override not restored: DefaultModel = %q", got)
	}
	if err := m2.SetDefaultModel(""); err != nil {
		t.Fatal(err)
	}
	if got := m2.DefaultModel(); got != "config/fallback" {
		t.Errorf("cleared override should fall back to config: %q", got)
	}
}
