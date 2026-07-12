# AxiOS

AI-native operating system built around `axiosd`, a model-agnostic intelligence
daemon. No provider is privileged: chat goes through a provider layer
(`pkg/providers`) with declarative per-vendor profiles over three wire transports
(OpenAI Chat Completions, Anthropic Messages, Ollama). Tools come from MCP servers
and a supervised background opencode coding agent, all gated by a tiered permission
model.

## Project structure

- `cmd/axiosd/` — daemon entry point (config load, provider resolution, wiring)
- `cmd/axios-fs/`, `cmd/axios-system/`, `cmd/axios-docker/`, `cmd/axios-ollama/`, `cmd/axios-network/`, `cmd/axios-git/` — active MCP servers; `cmd/axios-{gpu,media}/` are stubs
- `internal/axiosd/` — daemon internals: HTTP/WS server, agentic chat loop,
  provider runtime, sessions, permission middleware + WebSocket approval flow,
  opencode manager/API/task store, cloud/local router, MCP lifecycle
- `internal/config/` — daemon config loading (`LoadDaemon`), legacy-field mapping
- `internal/dockerctl/` — shared Docker CLI wrapper (daemon REST handlers + axios-docker MCP server)
- `internal/ollamactl/` — shared Ollama HTTP API client (daemon model-management handlers + axios-ollama MCP server)
- `internal/netctl/` — network inspection (interfaces, DNS, ping) + tailscale CLI wrapper (axios-network MCP server)
- `internal/gitctl/` — git CLI wrapper with strict path/ref/clone-URL validation (axios-git MCP server)
- `pkg/providers/` — provider profiles + registry, transports, error classifier,
  model-name normalization; canonical message format is the OpenAI Chat
  Completions shape
- `pkg/secrets/` — AES-256-GCM credential encryption at rest (`axsec1:` prefix,
  master key at `$AXIOS_DATA_DIR/master.key`)
- `pkg/permissions/` — tiered trust model, version-2 schema; pure (no I/O in `Check`)
- `pkg/opencode/` — HTTP+SSE client for `opencode serve`, pinned to the opencode
  v1.17.0 legacy API (re-diff `GET /doc` before upgrading opencode)
- `pkg/mcp/` — MCP protocol scaffold (Unix-socket server/client; enforces
  registered permission tiers as defense-in-depth)
- `pkg/logging/` — structured logging (slog) helpers
- `web/` — React + TypeScript + Tailwind web UI (Vite)
- `configs/` — runtime configs (`axiosd.yaml`, `permissions.yaml`, `firstboot/`)
- `systemd/` — `axiosd.service` unit; `scripts/axios` — dev start/stop
- `yocto/` — Yocto layer for the future bootable image
- `test/` — integration & E2E tests

## Build & test

- `make build` — all Go binaries into `bin/`
- `make vet` — `go vet ./...`
- `make test` — `go test ./... -race` (CI parity); `make test-web` for the UI
- Dev: `make dev-mcp` (MCP servers), `make dev-axiosd` (daemon on
  `127.0.0.1:3000`, uses `configs/axiosd.yaml`), `make dev-web` (Vite on 5173),
  or `./scripts/axios start` for everything
- Web: `cd web && npm install && npm run dev`; production build via `make web`
- CI (`.github/workflows/ci.yml`): `go vet ./...`, `go build ./...`,
  `go test ./... -race`, plus `npm ci && npm run build` in `web/`

## Conventions

- Go: standard layout — `cmd/` entry points, `internal/` private code, `pkg/`
  shared packages; Go 1.25, `go.work` at root
- Canonical chat format: `providers.Message` (OpenAI Chat Completions shape);
  transports convert at the wire boundary only; protocol replay state rides in
  `ProviderData` keyed by transport
- No hardcoded provider URLs outside `pkg/providers/builtin.go`; `http.Client`
  is injected everywhere (testable with `httptest`)
- Tool names: `server__tool` (e.g. `axios-fs__read_file`); `opencode` is a
  pseudo-server routed to the manager instead of an MCP socket
- Permissions: enforced by the daemon's `executeTool` middleware on every
  model-initiated call, and again inside MCP servers (defense in depth);
  unknown tools default to `approval_required`; always fail closed
- Secrets: never log key material or plaintext; encrypted values carry the
  `axsec1:` prefix; legacy base64 values are upgraded transparently on save
- MCP transport: Unix sockets, default `/tmp/axios-mcp` (config `mcp.socket_dir`)
- Runtime state: `$AXIOS_DATA_DIR` (default `~/.axios`) — `master.key`,
  `providers.json`, `hosts.json`, `sessions.json`, `opencode_tasks.json`
- Tests: table-driven, `_test.go` beside source, `httptest` for HTTP;
  integration tests in `test/`
- Design spec: `docs/phase1-design.md` is the authoritative Phase 1 reference
