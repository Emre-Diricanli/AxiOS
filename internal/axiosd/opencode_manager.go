package axiosd

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/axios-os/axios/pkg/opencode"
	"github.com/axios-os/axios/pkg/permissions"
	"github.com/axios-os/axios/pkg/providers"
)

// opencodeServerName is the pseudo server prefix for built-in opencode tools
// ("opencode__delegate_task"); executeTool routes it to the manager instead
// of an MCP socket.
const opencodeServerName = "opencode"

// opencodePermissionConfig is the opencode equivalent of a
// dangerously-skip-permissions mode. The installed opencode serve command
// has no CLI flag for this, so its supported permission config is used.
const opencodePermissionConfig = `{"$schema":"https://opencode.ai/config.json","permission":{"*":"allow"}}`

const (
	opencodeReadyPollInterval = 500 * time.Millisecond
	opencodeReadyTimeout      = 30 * time.Second
	opencodeRestartMinBackoff = time.Second
	opencodeRestartMaxBackoff = 60 * time.Second
	// opencodeHealthyRunReset: a process that survived this long resets the
	// restart backoff (the previous crash was not a crash loop).
	opencodeHealthyRunReset = 5 * time.Minute
	opencodeStopGracePeriod = 10 * time.Second
)

// opencodeAPI abstracts pkg/opencode.Client so tests can fake the server.
type opencodeAPI interface {
	Health() error
	CreateSession(dir, title string) (*opencode.Session, error)
	PromptAsync(sessionID string, model *opencode.ModelRef, text string) error
	Messages(sessionID string) ([]opencode.Message, error)
	ReplyPermission(sessionID, permID, response string) error
	ReplyQuestion(requestID string, answers [][]string) error
	RejectQuestion(requestID string) error
	Abort(sessionID string) error
	Diff(sessionID string) ([]opencode.FileDiff, error)
	Providers() ([]opencode.ProviderModels, error)
	Events(ctx context.Context) (<-chan opencode.Event, error)
}

// approvalFunc asks the user to approve an opencode-initiated action through
// the chat approval_request flow. It blocks until a verdict, timeout, or ctx
// cancellation; false means deny.
type approvalFunc func(ctx context.Context, tool string, params json.RawMessage) bool

// OpencodeOptions mirrors the config `opencode:` block (kept as a local type
// so internal/axiosd does not depend on internal/config).
type OpencodeOptions struct {
	Enabled   bool
	Binary    string // executable name or path, default "opencode"
	Port      int    // HTTP port for `opencode serve`, default 4097
	Workspace string // working directory for delegated tasks, default ~/axios-workspace
	// Model is the default "provider/model" for delegated tasks (e.g.
	// "xai/grok-build-0.1" to run on a SuperGrok subscription). Empty uses
	// opencode's own default.
	Model string
}

// OpencodeManager supervises a background `opencode serve` process and
// bridges it into the daemon: delegated coding tasks (chat tools + REST),
// task lifecycle tracking, and opencode permission asks resolved through the
// AxiOS permission policy.
type OpencodeManager struct {
	opts          OpencodeOptions
	providerStore *ProviderStore
	tasks         *opencodeTaskStore
	logger        *slog.Logger

	// Bound by Server.SetOpencodeManager.
	check   func(toolName string, args map[string]any) permissions.Tier
	approve approvalFunc

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.Mutex
	client   opencodeAPI // set in Start (or injected by tests)
	proc     *exec.Cmd
	ready    bool
	disabled bool   // runtime-disabled: config off or binary missing
	password string // HTTP Basic credential for this boot
	// modelOverride is the runtime default model for delegated tasks, set
	// via the API and persisted so it survives restarts. It wins over
	// opts.Model; "" means no override.
	modelOverride string
	// chatModel, when set (e.g. "xai/grok-4.5"), routes ALL chat turns
	// through the opencode bridge on that model — this is how a SuperGrok
	// model selected in the UI model picker becomes the chat backend.
	chatModel    string
	settingsPath string // "" = in-memory only (tests)
	// chat is the interactive code-chat bridge state (lazy; see codeChat()).
	chat *codeChatState
}

// opencodeSettings is the persisted shape of opencode_settings.json.
type opencodeSettings struct {
	Model     string `json:"model,omitempty"`
	ChatModel string `json:"chat_model,omitempty"`
}

// NewOpencodeManager creates a manager. tasksPath is where delegated-task
// state persists (empty = in-memory only). Call Server.SetOpencodeManager
// before Start so the permission bridge is bound.
func NewOpencodeManager(opts OpencodeOptions, ps *ProviderStore, tasksPath string, logger *slog.Logger) *OpencodeManager {
	if logger == nil {
		logger = slog.Default()
	}
	if opts.Binary == "" {
		opts.Binary = "opencode"
	}
	if opts.Port == 0 {
		opts.Port = 4097
	}
	settingsPath := ""
	if tasksPath != "" {
		settingsPath = filepath.Join(filepath.Dir(tasksPath), "opencode_settings.json")
	}
	m := &OpencodeManager{
		opts:          opts,
		providerStore: ps,
		tasks:         newOpencodeTaskStore(tasksPath, logger),
		logger:        logger,
		settingsPath:  settingsPath,
		// Fail closed until Server binds the real policy.
		check:   func(string, map[string]any) permissions.Tier { return permissions.Prohibited },
		approve: func(context.Context, string, json.RawMessage) bool { return false },
	}
	m.loadSettings()
	return m
}

// loadSettings restores the runtime model override.
func (m *OpencodeManager) loadSettings() {
	if m.settingsPath == "" {
		return
	}
	data, err := os.ReadFile(m.settingsPath)
	if err != nil {
		return // missing file is the common case
	}
	var s opencodeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		m.logger.Warn("failed to parse opencode settings", "path", m.settingsPath, "error", err)
		return
	}
	m.modelOverride = s.Model
	m.chatModel = s.ChatModel
}

// saveSettings persists the full runtime settings (caller must NOT hold m.mu).
func (m *OpencodeManager) saveSettings() error {
	m.mu.Lock()
	s := opencodeSettings{Model: m.modelOverride, ChatModel: m.chatModel}
	path := m.settingsPath
	m.mu.Unlock()
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// DefaultModel returns the effective default model for delegated tasks:
// the runtime override, else the config value, else "" (opencode's default).
func (m *OpencodeManager) DefaultModel() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.modelOverride != "" {
		return m.modelOverride
	}
	return m.opts.Model
}

// SetDefaultModel sets (or clears, with "") the runtime default model and
// persists it. Non-empty values must be "provider/model".
func (m *OpencodeManager) SetDefaultModel(model string) error {
	if model != "" && parseModelRef(model) == nil {
		return fmt.Errorf("model must be \"provider/model\", got %q", model)
	}
	m.mu.Lock()
	m.modelOverride = model
	m.mu.Unlock()
	return m.saveSettings()
}

// ChatModel returns the model chat turns are pinned to via the opencode
// bridge ("" = chat uses the provider layer).
func (m *OpencodeManager) ChatModel() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chatModel
}

// SetChatModel pins chat turns to an opencode-served model (or clears the
// pin with ""). Non-empty values must be "provider/model".
func (m *OpencodeManager) SetChatModel(model string) error {
	if model != "" && parseModelRef(model) == nil {
		return fmt.Errorf("model must be \"provider/model\", got %q", model)
	}
	m.mu.Lock()
	m.chatModel = model
	m.mu.Unlock()
	return m.saveSettings()
}

// AvailableModels lists the providers/models the running opencode server can
// actually use (working credentials), as "provider/model" ids.
func (m *OpencodeManager) AvailableModels() ([]string, error) {
	if err := m.available(); err != nil {
		return nil, err
	}
	provs, err := m.client.Providers()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range provs {
		for _, model := range p.Models {
			out = append(out, p.ID+"/"+model)
		}
	}
	return out, nil
}

// bind connects the manager to the daemon's permission policy and approval
// flow. Called by Server.SetOpencodeManager.
func (m *OpencodeManager) bind(check func(string, map[string]any) permissions.Tier, approve approvalFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if check != nil {
		m.check = check
	}
	if approve != nil {
		m.approve = approve
	}
}

// Start launches and supervises the opencode server. A disabled config or a
// missing binary turns the feature off without failing the daemon.
func (m *OpencodeManager) Start() error {
	if !m.opts.Enabled {
		m.setDisabled()
		m.logger.Info("opencode integration disabled in config")
		return nil
	}
	binPath, err := exec.LookPath(m.opts.Binary)
	if err != nil {
		m.setDisabled()
		m.logger.Warn("opencode binary not found — background coding agent disabled",
			"binary", m.opts.Binary, "error", err)
		return nil
	}

	workspace, err := m.resolveWorkspace()
	if err != nil {
		m.setDisabled()
		m.logger.Warn("opencode workspace unavailable — background coding agent disabled",
			"workspace", m.opts.Workspace, "error", err)
		return nil
	}
	m.opts.Workspace = workspace

	password, err := newOpencodePassword()
	if err != nil {
		m.setDisabled()
		m.logger.Error("failed to generate opencode server password — background coding agent disabled", "error", err)
		return nil
	}

	m.mu.Lock()
	m.password = password
	if m.client == nil { // tests inject a fake before Start
		m.client = opencode.NewClient(fmt.Sprintf("http://127.0.0.1:%d", m.opts.Port), password, nil)
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.mu.Unlock()

	m.wg.Add(2)
	go m.superviseLoop(binPath)
	go m.eventLoop()

	m.logger.Info("opencode manager starting",
		"binary", binPath, "port", m.opts.Port, "workspace", workspace)
	return nil
}

// Stop terminates the supervised process (SIGTERM, then SIGKILL after the
// grace period) and waits for the supervision goroutines to exit.
func (m *OpencodeManager) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	proc := m.proc
	m.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	if proc != nil && proc.Process != nil {
		_ = proc.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_, _ = proc.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(opencodeStopGracePeriod):
			m.logger.Warn("opencode server did not exit after SIGTERM — killing")
			_ = proc.Process.Kill()
		}
	}
	m.wg.Wait()
}

// Enabled reports whether the integration is live (config on, binary found).
func (m *OpencodeManager) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.disabled
}

// RestartIfIdle terminates the opencode server so the supervisor respawns it
// with fresh state (e.g. newly stored OAuth credentials) — but only when no
// delegated task is in flight, so running work is never killed mid-task.
func (m *OpencodeManager) RestartIfIdle() {
	for _, t := range m.tasks.list() {
		if !t.Status.terminal() {
			m.logger.Info("opencode restart skipped — tasks in flight; new credentials apply on next natural restart")
			return
		}
	}
	m.mu.Lock()
	proc := m.proc
	m.mu.Unlock()
	if proc == nil || proc.Process == nil {
		return
	}
	m.logger.Info("restarting opencode server to pick up new credentials")
	_ = proc.Process.Signal(syscall.SIGTERM)
}

func (m *OpencodeManager) setDisabled() {
	m.mu.Lock()
	m.disabled = true
	m.mu.Unlock()
}

func (m *OpencodeManager) isReady() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ready
}

func (m *OpencodeManager) setReady(v bool) {
	m.mu.Lock()
	m.ready = v
	m.mu.Unlock()
}

// resolveWorkspace expands and creates the delegated-task workspace.
func (m *OpencodeManager) resolveWorkspace() (string, error) {
	ws := m.opts.Workspace
	if ws == "" {
		ws = "~/axios-workspace"
	}
	if strings.HasPrefix(ws, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		ws = filepath.Join(home, strings.TrimPrefix(ws, "~"))
	}
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return "", err
	}
	return ws, nil
}

// superviseLoop spawns `opencode serve` and restarts it with exponential
// backoff until the manager is stopped.
func (m *OpencodeManager) superviseLoop(binPath string) {
	defer m.wg.Done()
	backoff := opencodeRestartMinBackoff
	for {
		started := time.Now()
		err := m.runOnce(binPath)
		m.setReady(false)
		if m.ctx.Err() != nil {
			return
		}
		if time.Since(started) > opencodeHealthyRunReset {
			backoff = opencodeRestartMinBackoff
		}
		m.logger.Warn("opencode server exited — restarting", "error", err, "backoff", backoff)
		select {
		case <-time.After(backoff):
		case <-m.ctx.Done():
			return
		}
		backoff = min(backoff*2, opencodeRestartMaxBackoff)
	}
}

// runOnce starts one opencode server process, waits for readiness, and blocks
// until the process exits.
func (m *OpencodeManager) runOnce(binPath string) error {
	cmd := exec.Command(binPath, opencodeServeArgs(m.opts.Port)...)
	cmd.Dir = m.opts.Workspace
	cmd.Env = buildOpencodeEnv(os.Environ(), m.password, opencodeCredentialEnv(m.providerStore))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	m.mu.Lock()
	m.proc = cmd
	m.mu.Unlock()

	go m.relayProcessLogs("stdout", stdout)
	go m.relayProcessLogs("stderr", stderr)
	go m.waitReady()

	err = cmd.Wait()
	m.mu.Lock()
	m.proc = nil
	m.mu.Unlock()
	return err
}

// waitReady polls /global/health until the server answers (or gives up).
func (m *OpencodeManager) waitReady() {
	deadline := time.Now().Add(opencodeReadyTimeout)
	for time.Now().Before(deadline) {
		if m.ctx.Err() != nil {
			return
		}
		if err := m.client.Health(); err == nil {
			m.setReady(true)
			m.logger.Info("opencode server ready", "port", m.opts.Port)
			return
		}
		select {
		case <-time.After(opencodeReadyPollInterval):
		case <-m.ctx.Done():
			return
		}
	}
	m.logger.Error("opencode server did not become healthy", "timeout", opencodeReadyTimeout)
}

// relayProcessLogs forwards the child's output to the daemon log at debug
// level (opencode is chatty; errors surface through the API/events instead).
func (m *OpencodeManager) relayProcessLogs(stream string, r interface{ Read([]byte) (int, error) }) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		m.logger.Debug("opencode", "stream", stream, "line", sc.Text())
	}
}

// eventLoop consumes the SSE event stream. pkg/opencode reconnects
// internally, so one subscription lives for the manager's lifetime.
func (m *OpencodeManager) eventLoop() {
	defer m.wg.Done()
	// Wait for the first readiness so the initial connect doesn't burn the
	// SSE backoff while the process is still booting.
	for !m.isReady() {
		select {
		case <-time.After(opencodeReadyPollInterval):
		case <-m.ctx.Done():
			return
		}
	}
	ch, err := m.client.Events(m.ctx)
	if err != nil {
		m.logger.Error("failed to subscribe to opencode events — permission bridge inactive", "error", err)
		return
	}
	for ev := range ch {
		m.handleEvent(ev)
	}
}

// handleEvent dispatches one SSE event. Permission asks run on their own
// goroutine: the approval wait can block for minutes and must not stall the
// stream.
func (m *OpencodeManager) handleEvent(ev opencode.Event) {
	switch ev.Type {
	case opencode.EventPermissionAsked:
		var p opencode.PermissionAsked
		if err := json.Unmarshal(ev.Properties, &p); err != nil {
			m.logger.Error("undecodable permission.asked event", "error", err)
			return
		}
		go m.handlePermissionAsked(p)
	case opencode.EventMessagePartDelta:
		m.handleCodeDelta(ev.Properties)
	case opencode.EventMessagePartUpdated:
		m.handleCodePartUpdated(ev.Properties)
	case opencode.EventQuestionAsked:
		m.handleCodeQuestion(ev.Properties)
	case opencode.EventSessionIdle:
		if sid := eventSessionID(ev.Properties); sid != "" {
			if m.isCodeSession(sid) {
				m.finishCodeTurn(sid, "")
			} else {
				m.finalizeTask(sid, "")
			}
		}
	case opencode.EventSessionError:
		sid := eventSessionID(ev.Properties)
		if sid == "" {
			return
		}
		if m.isCodeSession(sid) {
			m.finishCodeTurn(sid, string(ev.Properties))
		} else {
			m.finalizeTask(sid, string(ev.Properties))
		}
	}
}

// eventSessionID extracts the sessionID field common to session.* events.
func eventSessionID(props json.RawMessage) string {
	var p struct {
		SessionID string `json:"sessionID"`
	}
	_ = json.Unmarshal(props, &p)
	return p.SessionID
}

// handlePermissionAsked resolves one opencode permission ask through the
// AxiOS policy: trusted → allow once, prohibited → reject, approval_required
// → the chat approval flow (deny on timeout / no client).
func (m *OpencodeManager) handlePermissionAsked(p opencode.PermissionAsked) {
	toolName := opencodeServerName + "__" + p.Permission

	// The type-specific metadata (bash command, webfetch URL, ...) doubles as
	// the args for path-pattern permission rules.
	args := map[string]any{}
	if len(p.Metadata) > 0 {
		_ = json.Unmarshal(p.Metadata, &args)
	}

	response := opencode.PermissionReject
	outcome := "rejected"
	tier := m.check(toolName, args)
	switch tier {
	case permissions.Trusted:
		response, outcome = opencode.PermissionOnce, "allowed"
	case permissions.Prohibited:
		// response stays reject
	default: // approval_required and anything unknown: ask, fail closed
		params, _ := json.Marshal(map[string]any{
			"permission": p.Permission,
			"patterns":   p.Patterns,
			"metadata":   p.Metadata,
			"session_id": p.SessionID,
		})
		if m.approve(m.ctx, toolName, params) {
			response, outcome = opencode.PermissionOnce, "approved"
		} else {
			outcome = "denied"
		}
	}

	m.logger.Info("opencode permission decision",
		"permission", p.Permission, "tier", string(tier), "outcome", outcome, "session", p.SessionID)
	if err := m.client.ReplyPermission(p.SessionID, p.ID, response); err != nil {
		m.logger.Error("failed to reply to opencode permission ask",
			"session", p.SessionID, "permission_id", p.ID, "error", err)
	}
}

// finalizeTask records the terminal state of the task owning an opencode
// session. errDetail non-empty means the session errored.
func (m *OpencodeManager) finalizeTask(sessionID, errDetail string) {
	task, ok := m.tasks.byOpencodeSession(sessionID)
	if !ok || task.Status.terminal() {
		return
	}

	var result string
	var cost float64
	var inTok, outTok int
	msgs, err := m.client.Messages(sessionID)
	if err != nil {
		m.logger.Warn("failed to fetch opencode session transcript", "session", sessionID, "error", err)
	} else {
		for _, msg := range msgs {
			if msg.Info.Role != "assistant" {
				continue
			}
			cost += msg.Info.Cost
			inTok += msg.Info.Tokens.Input
			outTok += msg.Info.Tokens.Output
			var texts []string
			for _, part := range msg.Parts {
				if part.Type == "text" && part.Text != "" {
					texts = append(texts, part.Text)
				}
			}
			if len(texts) > 0 {
				result = strings.Join(texts, "\n") // last assistant message wins
			}
		}
	}

	now := time.Now()
	m.tasks.update(task.ID, func(t *OpencodeTask) {
		if errDetail != "" {
			t.Status = TaskFailed
			t.Error = errDetail
		} else {
			t.Status = TaskDone
		}
		t.CompletedAt = &now
		t.Result = result
		t.CostUSD = cost
		t.InputTokens = inTok
		t.OutputTokens = outTok
	})
	m.logger.Info("opencode task finished", "task", task.ID, "session", sessionID,
		"status", map[bool]string{true: string(TaskFailed), false: string(TaskDone)}[errDetail != ""])
}

// available returns a user-facing error when the integration cannot take work.
func (m *OpencodeManager) available() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disabled {
		return fmt.Errorf("opencode integration disabled")
	}
	if !m.ready {
		return fmt.Errorf("opencode server is not ready yet — try again shortly")
	}
	return nil
}

// Delegate creates an opencode session in dir (default: the workspace) and
// submits prompt asynchronously; progress lands in the task store via events.
// A nil model falls back to the configured default (opts.Model), then to
// opencode's own default.
func (m *OpencodeManager) Delegate(prompt, dir string, model *opencode.ModelRef) (OpencodeTask, error) {
	if strings.TrimSpace(prompt) == "" {
		return OpencodeTask{}, fmt.Errorf("prompt must not be empty")
	}
	if model == nil {
		if def := m.DefaultModel(); def != "" {
			model = parseModelRef(def)
		}
	}
	if err := m.available(); err != nil {
		return OpencodeTask{}, err
	}
	if dir == "" {
		dir = m.opts.Workspace
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return OpencodeTask{}, fmt.Errorf("directory does not exist: %s", dir)
	}

	sess, err := m.client.CreateSession(dir, taskTitle(prompt))
	if err != nil {
		return OpencodeTask{}, fmt.Errorf("failed to create opencode session: %w", err)
	}

	task := &OpencodeTask{
		ID:        newOpencodeTaskID(),
		SessionID: sess.ID,
		Prompt:    prompt,
		Directory: dir,
		Status:    TaskRunning,
		CreatedAt: time.Now(),
	}
	m.tasks.add(task)

	if err := m.client.PromptAsync(sess.ID, model, prompt); err != nil {
		now := time.Now()
		m.tasks.update(task.ID, func(t *OpencodeTask) {
			t.Status = TaskFailed
			t.Error = err.Error()
			t.CompletedAt = &now
		})
		return OpencodeTask{}, fmt.Errorf("failed to submit opencode task: %w", err)
	}

	m.logger.Info("opencode task delegated", "task", task.ID, "session", sess.ID, "dir", dir)
	return *task, nil
}

// Task returns one delegated task by id.
func (m *OpencodeManager) Task(id string) (OpencodeTask, bool) {
	return m.tasks.get(id)
}

// Tasks lists delegated tasks, most recent first.
func (m *OpencodeManager) Tasks() []OpencodeTask {
	return m.tasks.list()
}

// AbortTask cancels a running task's opencode session.
func (m *OpencodeManager) AbortTask(id string) error {
	task, ok := m.tasks.get(id)
	if !ok {
		return fmt.Errorf("unknown task: %s", id)
	}
	if task.Status.terminal() {
		return fmt.Errorf("task %s already %s", id, task.Status)
	}
	if err := m.client.Abort(task.SessionID); err != nil {
		return fmt.Errorf("failed to abort opencode session: %w", err)
	}
	now := time.Now()
	m.tasks.update(id, func(t *OpencodeTask) {
		t.Status = TaskAborted
		t.CompletedAt = &now
	})
	return nil
}

// TaskDiff returns the file changes a task's session accumulated.
func (m *OpencodeManager) TaskDiff(id string) ([]opencode.FileDiff, error) {
	task, ok := m.tasks.get(id)
	if !ok {
		return nil, fmt.Errorf("unknown task: %s", id)
	}
	return m.client.Diff(task.SessionID)
}

// --- chat tools -------------------------------------------------------------

// opencodeToolDefs are the built-in chat tools for delegating coding tasks.
// They ride the same permission pipeline as MCP tools (approval_required by
// default via the "opencode__*" policy entry).
func opencodeToolDefs() []providers.ToolDef {
	return []providers.ToolDef{
		{
			Name: opencodeServerName + "__delegate_task",
			Description: "Delegate a coding task to the background opencode agent. It works autonomously " +
				"in the given directory (default: the AxiOS workspace) and reports back asynchronously. " +
				"Returns a task id — check progress with opencode__task_status.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt":    map[string]any{"type": "string", "description": "The coding task to perform"},
					"directory": map[string]any{"type": "string", "description": "Absolute path of the project directory (optional)"},
				},
				"required": []any{"prompt"},
			},
		},
		{
			Name:        opencodeServerName + "__task_status",
			Description: "Check the status and result of a previously delegated opencode coding task.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{"type": "string", "description": "The task id returned by opencode__delegate_task"},
				},
				"required": []any{"task_id"},
			},
		},
	}
}

// ExecuteChatTool runs one of the built-in opencode chat tools (permission
// middleware has already allowed the call).
func (m *OpencodeManager) ExecuteChatTool(toolName string, params map[string]any) string {
	switch toolName {
	case "delegate_task":
		prompt, _ := params["prompt"].(string)
		dir, _ := params["directory"].(string)
		task, err := m.Delegate(prompt, dir, nil)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		return fmt.Sprintf("Task %s delegated (status: %s, directory: %s). Check progress with opencode__task_status.",
			task.ID, task.Status, task.Directory)

	case "task_status":
		id, _ := params["task_id"].(string)
		task, ok := m.Task(id)
		if !ok {
			return fmt.Sprintf("error: unknown task: %s", id)
		}
		out, err := json.MarshalIndent(task, "", "  ")
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		return string(out)

	default:
		return fmt.Sprintf("error: unknown opencode tool: %s", toolName)
	}
}

// --- pure builders (unit-tested without spawning processes) -----------------

// opencodeServeArgs builds the argv for `opencode serve`, bound to loopback.
func opencodeServeArgs(port int) []string {
	return []string{"serve", "--port", strconv.Itoa(port), "--hostname", "127.0.0.1", "--print-logs", "--log-level", "INFO"}
}

// buildOpencodeEnv assembles the child environment: the daemon's environment
// plus the server password, the permission-bypass config, and the decrypted
// provider credentials opencode needs to call models.
func buildOpencodeEnv(base []string, password string, creds map[string]string) []string {
	env := make([]string, 0, len(base)+len(creds)+2)
	env = append(env, base...)
	env = append(env, "OPENCODE_SERVER_PASSWORD="+password)
	env = append(env, "OPENCODE_CONFIG_CONTENT="+opencodePermissionConfig)
	for k, v := range creds {
		env = append(env, k+"="+v)
	}
	return env
}

// opencodeCredentialEnv maps each configured provider's primary env var to
// its decrypted key so the child process can authenticate with the same
// providers the daemon uses.
func opencodeCredentialEnv(ps *ProviderStore) map[string]string {
	out := map[string]string{}
	if ps == nil {
		return out
	}
	for _, profile := range providers.List() {
		if len(profile.EnvVars) == 0 {
			continue
		}
		if key, ok := ps.Credential(profile.Name); ok && key != "" {
			out[profile.EnvVars[0]] = key
		}
	}
	return out
}

// newOpencodePassword returns the per-boot HTTP Basic credential (32 hex chars).
func newOpencodePassword() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// newOpencodeTaskID returns a random task id ("oct-" + 12 hex chars).
func newOpencodeTaskID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("axiosd: crypto/rand failed: " + err.Error())
	}
	return "oct-" + hex.EncodeToString(b[:])
}

// taskTitle derives a short session title from the prompt.
func taskTitle(prompt string) string {
	title := strings.Join(strings.Fields(prompt), " ")
	if len(title) > 80 {
		title = title[:77] + "..."
	}
	return title
}
