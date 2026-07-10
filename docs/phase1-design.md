# Phase 1 Design — Model-Agnostic Core, Hardening, opencode Integration

Status: approved direction (July 10, 2026). This document is the single source of truth
for the Phase 1 refactor. Implementation agents: follow it exactly; where it is silent,
match existing repo conventions.

## Goals

1. **Model-agnostic daemon.** No provider is privileged. Claude/Anthropic becomes one
   `ProviderProfile` among many. Rename `claused` → `axiosd`.
2. **Provider layer modeled on Nous Research's hermes-agent** (two-layer design:
   declarative profiles + wire-protocol transports over one canonical message format).
3. **Permission enforcement actually wired** — tiered trust checked on every
   model-initiated tool call, with a WebSocket approval flow.
4. **Credentials encrypted at rest** (AES-GCM) instead of base64.
5. **opencode as a managed background coding agent** — the daemon supervises
   `opencode serve` and delegates self-coding / machine-control tasks to it, bridging
   opencode's permission asks into our approval flow.
6. **First tests + CI.** The repo currently has zero `*_test.go` files.

Out of scope for Phase 1: full HTTP authn/authz on REST endpoints (we only flip the
default bind to 127.0.0.1 and document it), Yocto image, credential-pool rotation,
models.dev metadata cache, the six stub MCP servers.

## Rename: claused → axiosd

- `internal/claused/` → `internal/axiosd/` (package `axiosd`), `cmd/claused/` → `cmd/axiosd/`.
- Config type `Claused`/`LoadClaused` in `internal/config/config.go` → `Daemon`/`LoadDaemon`.
- Default config path `/etc/axios/axiosd.yaml`; if absent, fall back to
  `/etc/axios/claused.yaml` (log a deprecation warning). Same for `configs/axiosd.yaml`.
- `systemd/claused.service` → `systemd/axiosd.service`, Description
  "AxiOS Intelligence Daemon", `ExecStart=/usr/bin/axiosd`.
- Makefile targets, `scripts/axios`, `.gitignore` updated; delete the stray compiled
  binaries at repo root (`claused`, `axios-fs`, `axios-system`).
- `configs/permissions.yaml` operation `system:modify_claused` → covered by new schema (below).
- Comments/docs mentioning claused updated in the files touched.

## Provider layer (new package `pkg/providers/`)

Canonical internal format is **OpenAI Chat Completions shape** (messages with `role`,
`content`, assistant `tool_calls`, `role:"tool"` results; tools as OpenAI
function-calling JSON schema). Transports convert at the wire boundary only.

```go
// pkg/providers/types.go
type Message struct {
    Role       string     `json:"role"` // system|user|assistant|tool
    Content    string     `json:"content,omitempty"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    ToolCallID string     `json:"tool_call_id,omitempty"` // for role:"tool"
    Name       string     `json:"name,omitempty"`
    // ProviderData holds protocol replay state (e.g. Anthropic signed thinking
    // blocks) keyed by transport; stripped before sending to a different transport.
    ProviderData map[string]json.RawMessage `json:"provider_data,omitempty"`
}
type ToolCall struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Arguments string `json:"arguments"` // JSON string
}
type ToolDef struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    InputSchema map[string]any `json:"input_schema"`
}
type Usage struct{ InputTokens, OutputTokens int }
type NormalizedResponse struct {
    Content      string
    Reasoning    string
    ToolCalls    []ToolCall
    FinishReason string // stop|tool_calls|length|content_filter
    Usage        Usage
    ProviderData map[string]json.RawMessage
}
```

```go
// pkg/providers/profile.go — declarative, one per provider (hermes ProviderProfile)
type Profile struct {
    Name          string
    Aliases       []string
    APIMode       string   // chat_completions|anthropic_messages|ollama
    BaseURL       string
    EnvVars       []string // credential env vars, priority order
    DefaultModel  string
    FallbackModels []string
    DefaultHeaders map[string]string
    // Hooks (nil = default behavior):
    PrepareRequest func(req map[string]any, model string) // last-mile quirks
    AuthHeader     func(key string) (header, value string) // default: Authorization: Bearer
}
```
Built-in profiles (registered in `pkg/providers/builtin.go`), matching today's catalog in
`providers.go:31-107`: anthropic (aliases: claude; api_mode anthropic_messages; auth
x-api-key vs Bearer by `sk-ant-oat01-` prefix; header `anthropic-version: 2023-06-01`),
openai, openrouter, groq, deepseek, xai, mistral, together, fireworks, google (via its
OpenAI-compatible endpoint), ollama (api_mode ollama, baseURL from HostStore), custom
(openai-compatible, user baseURL). Registry: `Register(p)`, `Get(nameOrAlias)`, `List()`;
last-writer-wins so tests/users can override.

```go
// pkg/providers/transport.go
type Transport interface {
    BuildRequest(ctx context.Context, p *Profile, apiKey, baseURL, model string,
        system string, msgs []Message, tools []ToolDef, stream bool) (*http.Request, error)
    ParseResponse(body io.Reader) (*NormalizedResponse, error)
    // ParseStream accumulates deltas (calling onDelta for text) and returns the
    // SAME NormalizedResponse shape as ParseResponse — one downstream code path.
    ParseStream(body io.Reader, onDelta func(text string)) (*NormalizedResponse, error)
}
func GetTransport(apiMode string) Transport
```
Three transports: `transport_openai.go` (finish_reason passthrough; tool_calls native),
`transport_anthropic.go` (convert msgs→content blocks incl. tool_use/tool_result;
stop_reason map: end_turn→stop, tool_use→tool_calls, max_tokens→length,
refusal→content_filter; preserve ordered content blocks in ProviderData
`anthropic_content_blocks` for replay), `transport_ollama.go` (native /api/chat; tool
support per current ollama.go conversion logic).

```go
// pkg/providers/client.go — the one concrete ChatProvider
type Client struct { /* profile, transport, httpClient, apiKey, baseURL, model */ }
func NewClient(profile *Profile, apiKey, baseURL, model string, hc *http.Client) *Client
func (c *Client) Complete(ctx, system string, msgs []Message, tools []ToolDef) (*NormalizedResponse, error)
func (c *Client) Stream(ctx, system string, msgs []Message, tools []ToolDef, onDelta func(string)) (*NormalizedResponse, error)
func (c *Client) Name() string; func (c *Client) Model() string; func (c *Client) SetModel(m string)
```
`http.Client` is injected (testable with `httptest`). No hardcoded URLs anywhere.

```go
// pkg/providers/errors.go — hermes-style classifier, scaled down
type Reason string // auth|billing|rate_limit|overloaded|server_error|timeout|
                   // context_overflow|model_not_found|content_policy|network|unknown
type ClassifiedError struct {
    Reason Reason; StatusCode int; Provider, Model, Message string
    Retryable bool; ShouldFallback bool
}
func Classify(err error, statusCode int, body []byte, provider, model string) *ClassifiedError
```
Retry policy (in the daemon, not the transport): up to 3 attempts with backoff for
`Retryable`; on `ShouldFallback`, advance the fallback chain (config
`fallback_providers: [{provider, model}]`), swapping client+model in place.

`pkg/providers/modelname.go`: `NormalizeModelForProvider(model, provider) string` —
aggregators (openrouter) want `vendor/model` slugs, native APIs want bare names.

## Daemon integration (`internal/axiosd/`)

- `Server` drops the three concrete client fields (`anthropic`, `openaiClient`; keep
  `ollama *OllamaClient` only for model-management/marketplace APIs — chat goes through
  the provider layer, including an ollama-profile Client).
- New `provider_runtime.go`: resolves the active `*providers.Client` from
  ProviderStore + HostStore; rebuilt on provider/model switch.
- **One agentic loop** (`chatloop.go`) replaces `handleCloudMessage` (server.go:415),
  `handleOpenAICloudMessage` (providers.go:592), and `handleLocalMessage`
  (server.go:523): max 20 iterations; on `finish_reason=="tool_calls"` execute tools via
  the (now permission-gated) executor, append canonical `role:"tool"` results, continue;
  stream text deltas to the sink when the transport supports it. Sink is an interface
  (`WriteJSON(any) error`) so the loop is testable without websockets. Assistant chat
  messages carry the real provider/model name (no more `Model:"claude"` literal —
  server.go:470).
- Sessions (`session.go`): store canonical `providers.Message`. Bump sessions.json to
  `{"version":2,...}`; on load, best-effort convert legacy Anthropic-block sessions
  (text blocks only; drop tool blocks) and log what was skipped. Write file 0600.
- Bootstrap (`cmd/axiosd/main.go`): no Anthropic-first logic. Resolution order:
  explicit `model.provider` in config → first registered profile whose env var is set
  (iterate registry deterministically: anthropic, openai, openrouter, …) → saved active
  provider in ProviderStore → local ollama if reachable → start with no provider (UI
  setup wizard handles it). Legacy `anthropic:` config section still read (deprecation
  warning) and mapped onto the new schema.
- Config (`internal/config/config.go`) new schema:
  ```yaml
  model:
    provider: auto        # auto | anthropic | openai | openrouter | ollama | custom...
    id: ""                # empty = provider default
  fallback_providers: []   # [{provider: ..., model: ...}]
  opencode:
    enabled: true
    binary: opencode
    port: 4097
    workspace: ~/axios-workspace
  server:
    listen: 127.0.0.1:3000   # was 0.0.0.0 — LAN exposure now requires explicit opt-in
  ```
- `/v1/chat/completions` (openai_compat.go): keep the facade, but route through the
  provider layer instead of the bespoke `handleV1ChatAnthropic*` translation.
- `/api/ai/ask`: route through the provider layer (delete aiAskAnthropic/aiAskOpenAI/aiAskOllama).

## Permissions (upgrade `pkg/permissions/`, wire into dispatch)

New schema (`configs/permissions.yaml`, version 2) keyed by **actual runtime tool names**
(`server__tool`) with `*`/`?` wildcards; evaluation order: prohibited → trusted →
approval_required; unknown tools default to `approval_required`:

```yaml
version: 2
default_tier: approval_required
prohibited:
  - "axios-fs__write_file:/etc/axios/*"     # tool:path-pattern form, optional
  - "axios-fs__read_file:~/.axios/providers.json"
  - "axios-fs__read_file:~/.axios/master.key"
trusted:
  - "axios-fs__read_file"
  - "axios-fs__list_directory"
  - "axios-fs__search_files"
  - "axios-fs__file_info"
  - "axios-system__system_info"
  - "axios-system__disk_usage"
  - "axios-system__process_list"
  - "axios-system__service_status"
approval_required:
  - "axios-fs__write_file"
  - "axios-fs__delete_file"
  - "axios-system__run_command"   # was 'trusted' in the MCP server — fix there too
  - "axios-system__reboot"
  - "opencode__*"
```
`Check(toolName string, args map[string]any) Tier`. The `tool:pattern` form matches the
primary path-like argument when present (keys: path, file, target); plain entries match
the tool name only. `pkg/permissions` stays pure (no I/O in Check) and fully unit-tested.

Wiring: `executeTool` (server.go:636) becomes a middleware chain:
`Check` → trusted: execute; prohibited: return an error ToolResult to the model (never
executes); approval_required: send WS `{"type":"approval_request","id":...,"tool":...,
"params":...}`, block on a pending-approvals map until
`{"type":"approval_response","id":...,"approve":bool}` or timeout (default 120s,
config `approval_timeout_seconds`) → timeout = deny. Also fix
`cmd/axios-system/main.go:46` (`run_command` self-declared tier trusted →
approval_required) and make `pkg/mcp/server.go` `handleToolCall` honor the registered
`ToolDefinition.Permission` as defense-in-depth (reject prohibited even if the daemon
is compromised into skipping its check). `BuildTools` (server.go:98) keeps dropping the
Permission field from model-facing defs (the model doesn't need to see tiers).

## Credentials at rest (new package `pkg/secrets/`)

Threat model (be honest in doc comments): protects `providers.json` against at-rest
exfiltration (backups, copied disks, other local users). It does NOT protect against a
compromised daemon or anything that can run code as the user — the key lives beside the
data. Keychain/TPM is a later upgrade.

- `pkg/secrets`: `NewStore(keyPath string)` — loads or creates a random 32-byte key at
  `$AXIOS_DATA_DIR/master.key` (0600). `Encrypt(plaintext []byte) (string, error)` /
  `Decrypt(s string) ([]byte, error)` — AES-256-GCM, random nonce prepended,
  base64-encoded, versioned prefix `axsec1:`.
- ProviderStore save/load uses the store; values without the `axsec1:` prefix are
  treated as legacy base64, decoded, and re-encrypted on next save (transparent upgrade).
- `getProviderWithKey` encapsulation leak (openai_compat.go:665) replaced with a proper
  accessor.

## opencode integration

New package `pkg/opencode/` (HTTP+SSE client, no exec) + `internal/axiosd/opencode_manager.go`
(process supervision) + `internal/axiosd/opencode_api.go` (REST) — all pinned to the
verified v1.17.0 legacy API surface.

Client (`pkg/opencode/client.go`, injected base URL + password, HTTP Basic
`opencode:<password>` on every request):
- `Health()` → GET `/global/health`
- `CreateSession(dir, title)` → POST `/session?directory=…`
- `PromptAsync(sessionID, model, text)` → POST `/session/{id}/prompt_async`
- `Messages(sessionID)` → GET `/session/{id}/message`
- `ReplyPermission(sessionID, permID, response)` → POST `/session/{sid}/permissions/{pid}`
  body `{"response":"once"|"always"|"reject"}`
- `Abort(sessionID)`, `DeleteSession(sessionID)`, `Status()` → GET `/session/status`,
  `Diff(sessionID)` → GET `/session/{id}/diff`
- `Events(ctx)` → GET `/event` SSE stream (client Timeout 0, ctx cancellation, 1MB+
  scanner buffer, auto-reconnect with backoff); parse `data: {"type":…,"properties":…}`
  frames. Handle event types: `session.idle`, `session.error`, `permission.asked`,
  `message.part.delta`, `question.asked`. Parse defensively — properties as
  `json.RawMessage` until switched on type.

Manager (`opencode_manager.go`):
- Spawn `opencode serve --port <cfg.port> --hostname 127.0.0.1 --print-logs --log-level INFO`,
  CWD = configured workspace, env: `OPENCODE_SERVER_PASSWORD` (random 32-hex per boot),
  decrypted provider API keys from ProviderStore (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, …
  per profile EnvVars), and `OPENCODE_CONFIG_CONTENT` with our locked-down config:
  ```json
  {"$schema":"https://opencode.ai/config.json",
   "permission":{"*":"ask","edit":"allow",
     "bash":{"*":"ask","git status*":"allow","git diff*":"allow","git log*":"allow",
             "go build*":"allow","go test*":"allow","go vet*":"allow","ls*":"allow",
             "rm -rf *":"deny","sudo *":"deny"},
     "webfetch":"ask"}}
  ```
  (model left unset — passed per-request from the active AxiOS provider when compatible,
  else opencode's own default).
- Readiness: poll Health every 500ms, 30s cap. Supervise: restart with exponential
  backoff on exit; SIGTERM then SIGKILL on daemon shutdown. Disabled cleanly when
  `opencode.enabled: false` or binary missing (log once, feature off — daemon must not fail).
- Event bridge: on `permission.asked` → run through `pkg/permissions` policy for tool
  `opencode__<permission-type>`; approval_required → forward to the UI as a standard
  `approval_request` WS message (over the chat session that spawned the task, or broadcast);
  reply `once`/`reject` accordingly; always reject on timeout.

Surface:
- REST: `POST /api/code/tasks` `{prompt, directory?, model?}` → `{task_id}`;
  `GET /api/code/tasks` (list w/ status), `GET /api/code/tasks/{id}` (status, result text,
  cost/tokens, diff), `DELETE /api/code/tasks/{id}` (abort). Task state in memory,
  persisted to `$AXIOS_DATA_DIR/opencode_tasks.json`.
- Built-in daemon tool `opencode__delegate_task` (registered alongside MCP tools in
  BuildTools; executed by the manager, not an MCP socket): the chat model can delegate
  a coding task and gets back the task id; a follow-up tool `opencode__task_status`
  fetches result. Both are `approval_required` by default (see permissions.yaml).

## Tests & CI

Required unit tests (table-driven, `httptest` where HTTP is involved):
- `pkg/providers`: each transport round-trip (canonical→wire→normalized), finish-reason
  maps, error classifier table, model-name normalization, registry override.
- `pkg/secrets`: encrypt/decrypt round-trip, legacy base64 upgrade, tamper detection,
  key file perms.
- `pkg/permissions`: tier evaluation incl. wildcards, arg-pattern form, unknown-tool
  default, prohibited precedence.
- `pkg/opencode`: client against fake `httptest` server (session create, prompt, perm
  reply), SSE parsing incl. reconnect and oversized frames.
- `internal/axiosd`: chat loop with fake provider + fake sink (tool-call iteration,
  max-iteration stop, fallback-chain advance), executeTool permission middleware
  (trusted/approval/prohibited/timeout paths), ProviderStore encryption round-trip
  (t.TempDir), router modes.
- Existing pure converters that survive the refactor get tests too.

CI (`.github/workflows/ci.yml`): on push/PR — Go 1.25.x: `go vet ./...`,
`go build ./...`, `go test ./... -race`; web job: `cd web && npm ci && npm run build`
(and `tsc --noEmit` if not part of build). Makefile: `make test` should now be real.

## Web UI (minimal Phase 1 changes only)

- Handle `approval_request` in the chat WebSocket handler: render an inline approval
  card (tool name, params, Approve/Deny buttons) → send `approval_response`.
- Display real provider/model name from assistant messages (no hardcoded Claude assumptions).
- Setup wizard: keep anthropic as an option but not pre-selected default; default to
  "choose a provider" state (SetupWizard.tsx:67,418,1177).

## Docs

README.md / CLAUDE.md: rebrand from "Claude IS the OS" to model-agnostic ("AxiOS —
the AI-native OS; bring your own model"), document the new config schema, provider
setup per-vendor, opencode integration, permission model (now real), and the
credential threat model. Note deployment target: dev on macOS today, Linux/Yocto later.
