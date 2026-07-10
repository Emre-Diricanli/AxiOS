# AxiOS

**The AI-native operating system — bring your own model.**

AxiOS (from Greek *axios*, meaning "worthy") is an AI-native environment built around
`axiosd`, a model-agnostic intelligence daemon. No provider is privileged: Anthropic,
OpenAI, OpenRouter, Groq, DeepSeek, xAI, Mistral, Together, Fireworks, Google, a local
Ollama instance, or any OpenAI-compatible endpoint — pick one, set one environment
variable, and the same daemon, tools, and permission model work identically.

> *Your hardware is worthy. Your creative work is worthy. This machine deserves
> intelligence — from whichever model you choose.*

---

## What is this?

AxiOS turns a machine into a conversational system: you talk to a model, and the model
operates the machine through tiered, permission-gated tools. The daemon manages MCP
servers for filesystem and system control, supervises a background coding agent
([opencode](https://opencode.ai)) for delegated programming tasks, and serves a web UI
over WebSocket. Model access goes through a provider layer with one canonical message
format, so switching vendors — or falling back from one to another mid-conversation —
requires no application changes.

### Core principles

1. **Model-agnostic** — every provider is just a `ProviderProfile`. Switching is a
   config value or an environment variable, never a code change.
2. **Local AI too** — Ollama is a first-class provider for offline operation and
   privacy-sensitive queries (`routing.mode: local_only` keeps data on the machine).
3. **Tiered trust, actually enforced** — every model-initiated tool call passes through
   permission middleware in the daemon; destructive actions require explicit user
   approval; prohibited actions never execute.
4. **Honest security** — credentials are encrypted at rest with a documented,
   deliberately narrow threat model (see below).
5. **Your machine, your data** — the daemon binds to loopback by default; LAN exposure
   is an explicit opt-in.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                             Clients                              │
│     Web UI (React/Vite, WebSocket)  ·  OpenAI-compatible API     │
│                (/v1/chat/completions, /v1/models)                │
├──────────────────────────────────────────────────────────────────┤
│                              axiosd                              │
│                  (AxiOS Intelligence Daemon, Go)                 │
│    agentic chat loop · sessions · cloud/local routing            │
│    permission middleware + WebSocket approval flow               │
│    provider runtime · MCP lifecycle · opencode supervisor        │
├──────────────────────┬──────────────────────┬────────────────────┤
│    pkg/providers     │     MCP servers      │  opencode agent    │
│  anthropic  openai   │  axios-fs            │  supervised        │
│  openrouter groq     │  axios-system        │  `opencode serve`  │
│  deepseek   xai      │  (Unix sockets;      │  background coding │
│  mistral    together │   stubs: docker gpu  │  tasks via chat    │
│  fireworks  google   │   media network git  │  tools + REST      │
│  ollama     custom   │   ollama)            │  (loopback only)   │
├──────────────────────┴──────────────────────┴────────────────────┤
│   pkg/secrets (AES-256-GCM at rest) · pkg/permissions (tiers)    │
├──────────────────────────────────────────────────────────────────┤
│            macOS (dev today)  →  Linux / Yocto (target)          │
└──────────────────────────────────────────────────────────────────┘
```

**`axiosd`** — the core daemon (Go). Resolves the active provider, runs the agentic
chat loop (streaming, tool calls, fallback chains), enforces the permission model,
manages MCP server lifecycle, supervises the opencode coding agent, and serves the
web UI plus an OpenAI-compatible facade.

**Provider layer (`pkg/providers`)** — a two-layer design modeled on Nous Research's
hermes-agent: declarative per-vendor profiles (base URL, credential env vars, default
model, auth quirks) on top of three wire transports (OpenAI Chat Completions, Anthropic
Messages, Ollama). The canonical internal message format is the OpenAI Chat Completions
shape; transports convert at the wire boundary only. A hermes-style error classifier
drives retries and provider fallback.

**MCP servers** — each exposes one domain of system control as tools the model can
call (`axios-fs` for files, `axios-system` for system info and commands), speaking MCP
over Unix sockets (default `/tmp/axios-mcp`, configurable via `mcp.socket_dir`).

**opencode agent** — a supervised background instance of `opencode serve` that handles
delegated coding tasks, with its permission asks bridged into the AxiOS approval flow.

**Web UI** — React + TypeScript + Tailwind (Vite). Chat-first interface with system
monitoring, terminal, and file browser; renders tool-approval requests inline.

---

## Providers

Built-in provider profiles and their credential environment variables (checked in
order; the first profile with a key set wins auto-selection at boot):

| Provider | Alias | Env var(s) | API mode |
|---|---|---|---|
| `anthropic` | `claude` | `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN` | Anthropic Messages |
| `openai` | | `OPENAI_API_KEY` | Chat Completions |
| `openrouter` | | `OPENROUTER_API_KEY` | Chat Completions |
| `groq` | | `GROQ_API_KEY` | Chat Completions |
| `deepseek` | | `DEEPSEEK_API_KEY` | Chat Completions |
| `xai` | `grok` | `XAI_API_KEY` | Chat Completions |
| `mistral` | | `MISTRAL_API_KEY` | Chat Completions |
| `together` | | `TOGETHER_API_KEY`, `TOGETHERAI_API_KEY` | Chat Completions |
| `fireworks` | | `FIREWORKS_API_KEY` | Chat Completions |
| `google` | `gemini` | `GEMINI_API_KEY`, `GOOGLE_API_KEY` | Chat Completions (OpenAI-compatible endpoint) |
| `ollama` | | none — local server | Ollama `/api/chat` |
| `custom` | | key + base URL via the provider store | Chat Completions |

Setup is the same for every vendor — export the key and start the daemon:

```bash
export OPENROUTER_API_KEY=sk-or-...   # or ANTHROPIC_API_KEY, OPENAI_API_KEY,
                                      # GROQ_API_KEY, DEEPSEEK_API_KEY, ...
make dev-axiosd
```

Notes:

- **Anthropic auth**: OAuth tokens from `claude setup-token` (prefix `sk-ant-oat01-`)
  authenticate with `Authorization: Bearer`; plain API keys use `x-api-key`. The
  profile picks the right header automatically.
- **Boot resolution order** (when `model.provider: auto`): explicit provider in config
  → first registered profile whose env var is set (deterministic registry order) →
  saved active provider in the provider store → reachable local Ollama → start
  unconfigured (the web UI setup wizard takes over).
- **Fallback chains**: when the active provider fails with a fallback-worthy error
  (auth, billing, rate limit, outage), the daemon walks the configured
  `fallback_providers` list, swapping provider and model in place.
- Keys can also be added at runtime through the web UI; they are persisted encrypted
  (see [Credentials at rest](#credentials-at-rest)).

---

## Configuration

Daemon config lives at `/etc/axios/axiosd.yaml` (dev checkouts use
`configs/axiosd.yaml` via `make dev-axiosd`). Current schema:

```yaml
server:
  # Bind address. Loopback-only by default — exposing axiosd on the LAN
  # requires explicit opt-in (e.g. "0.0.0.0:3000").
  listen: "127.0.0.1:3000"

model:
  # auto: resolve at boot — explicit provider here, then the first provider
  # whose API-key env var is set, then the saved active provider, then a
  # reachable local Ollama, otherwise start unconfigured (setup wizard).
  # Or name a provider explicitly: anthropic | openai | openrouter | groq |
  # deepseek | xai | mistral | together | fireworks | google | ollama | custom
  provider: auto
  # Model ID. Empty = the provider's default model.
  id: ""

# Ordered chain tried when the active provider fails with a fallback-worthy
# error (auth, billing, rate limit, outage, ...).
# fallback_providers:
#   - provider: openrouter
#     model: anthropic/claude-sonnet-4
#   - provider: ollama
#     model: llama3.1:8b
fallback_providers: []

opencode:
  # Managed background coding agent (opencode serve), supervised by axiosd.
  enabled: true
  binary: opencode
  port: 4097
  workspace: ~/axios-workspace

ollama:
  enabled: true
  host: "localhost"
  port: 11434
  model: "llama3.1:8b"

permissions:
  # Tiered-trust policy enforced on every model-initiated tool call.
  # Falls back to the repo's configs/permissions.yaml (dev checkouts) and
  # then to the built-in default policy when this file is missing.
  path: /etc/axios/permissions.yaml
  # How long an approval_required tool call waits for the user's
  # approval_response before being denied.
  approval_timeout_seconds: 120

routing:
  # auto: use cloud when available, fallback to local
  # cloud_only: only use the active cloud provider
  # local_only: only use Ollama (no data leaves the machine)
  # cost_aware: track API spend, shift to local when budget depletes
  mode: "auto"

mcp:
  socket_dir: "/tmp/axios-mcp"
  servers:
    - axios-fs
    - axios-system
```

Runtime state lives in `$AXIOS_DATA_DIR` (default `~/.axios`): `master.key`,
`providers.json` (encrypted credentials + active provider), `hosts.json`,
`sessions.json`, and `opencode_tasks.json`.

---

## Permission model

AxiOS uses a three-tier trust system, defined in `configs/permissions.yaml`
(version 2) and enforced by the daemon on **every** model-initiated tool call —
not by the model, and not by prompt engineering.

Entries are keyed by actual runtime tool names (`server__tool`) and support `*`/`?`
wildcards. The optional `tool:pattern` form additionally matches the call's primary
path-like argument (keys: `path`, `file`, `target`; `~` expands to the home
directory). Evaluation order: **prohibited → trusted → approval_required →
default tier**; unknown tools default to `approval_required` (fail closed).

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
  - "axios-system__run_command"
  - "axios-system__reboot"
  - "opencode__*"
```

How each tier behaves at execution time:

| Tier | Behavior |
|------|----------|
| **Trusted** | Executes immediately. |
| **Approval required** | The daemon sends `{"type":"approval_request","id":...,"tool":...,"params":...}` over the chat WebSocket and blocks until the client answers `{"type":"approval_response","id":...,"approve":...}`. Timeout (`permissions.approval_timeout_seconds`, default 120s), disconnect, or an explicit deny all resolve to **deny**; the model receives an error tool result and the conversation continues. |
| **Prohibited** | Never executes. The model gets an error tool result; the daemon logs the block. |

**opencode permission bridge** — the background coding agent has its own permission
asks (`bash`, `edit`, `webfetch`, ...). Each `permission.asked` event from opencode is
mapped to the tool name `opencode__<permission-type>` and run through the same policy:
trusted → allowed once, prohibited → rejected, approval_required → forwarded to the
user as a standard `approval_request`. Timeouts always reject.

**Defense in depth** — MCP servers independently honor the permission tier each tool
was registered with: `pkg/mcp` refuses to execute a tool registered as `prohibited`
even if a compromised daemon were to skip its own check, and `axios-system` declares
`run_command` and `reboot` as `approval_required` at the source. Permission tiers are
stripped from the tool definitions sent to the model — the model never sees (or
negotiates) trust levels.

---

## Credentials at rest

Provider API keys in `providers.json` are encrypted with AES-256-GCM (`pkg/secrets`;
values carry an `axsec1:` version prefix, random nonce per encryption, authenticated
tags). The 32-byte master key is generated on first boot at
`$AXIOS_DATA_DIR/master.key` with mode 0600. Legacy plain-base64 values are decoded
and transparently re-encrypted on the next save.

**Honest threat model:** this protects `providers.json` against *at-rest* exfiltration
— backups, copied disks, and other local users reading the file. It does **not**
protect against a compromised daemon or anything that can run code as the AxiOS user,
because the master key lives on disk beside the data it protects. Keychain/TPM-backed
key storage is a planned later upgrade; treat filesystem permissions and daemon
integrity as the real boundary today.

---

## opencode integration

`axiosd` supervises an [opencode](https://opencode.ai) server as a background coding
agent. Coding tasks can be delegated two ways:

- **From chat** — the model calls the built-in `opencode__delegate_task` tool
  (`approval_required` by default) and gets back a task id; `opencode__task_status`
  fetches progress and results. These are daemon-built-in tools that ride the same
  permission pipeline as MCP tools.
- **Over REST**:

  | Endpoint | Description |
  |---|---|
  | `POST /api/code/tasks` | `{prompt, directory?, model?}` → `{task_id}` (model as `provider/model`, optional) |
  | `GET /api/code/tasks` | List delegated tasks with status |
  | `GET /api/code/tasks/{id}` | Status, result text, cost/tokens, and the file diff once finished |
  | `DELETE /api/code/tasks/{id}` | Abort a running task |

**Supervision** — the daemon spawns `opencode serve` bound to `127.0.0.1` on the
configured port, authenticated with a random per-boot HTTP Basic password. It restarts
the process with exponential backoff on crashes, and shuts it down with SIGTERM (then
SIGKILL) when the daemon stops. If `opencode.enabled: false` or the binary is missing,
the feature turns off cleanly — the daemon never fails because of it.

**Locked-down permissions** — the managed server always starts with a restrictive
config injected via `OPENCODE_CONFIG_CONTENT`: safe build/inspect commands
(`git status/diff/log`, `go build/test/vet`, `ls`) are allowed, destructive commands
(`rm -rf`, `sudo`) are denied outright, and everything else surfaces as a permission
ask that is resolved through the AxiOS permission bridge (see above). The daemon also
passes its decrypted provider credentials to the child process, so opencode uses the
same providers you configured — no separate credential setup.

**Version pin** — `pkg/opencode` speaks the legacy API surface verified against
**opencode v1.17.0** (session create/prompt/abort, permission replies, the `/event`
SSE stream — not the newer `/api/*` routes). Before upgrading the opencode binary,
re-diff its `GET /doc` OpenAPI schema against `pkg/opencode` and adjust the client.

---

## Project structure

```
AxiOS/
├── cmd/                    # Go binary entry points
│   ├── axiosd/             # Core daemon
│   ├── axios-fs/           # MCP: filesystem
│   ├── axios-system/       # MCP: system info & control
│   └── axios-{docker,gpu,media,network,git,ollama}/   # MCP stubs (future)
├── internal/
│   ├── axiosd/             # Daemon internals: server, chat loop, provider
│   │                       # runtime, sessions, permission middleware + WS
│   │                       # approvals, opencode manager/API, routing
│   └── config/             # Daemon config loading (incl. legacy mapping)
├── pkg/
│   ├── providers/          # Provider profiles, transports, error classifier
│   ├── secrets/            # AES-256-GCM credential encryption at rest
│   ├── permissions/        # Tiered trust model (pure, unit-tested)
│   ├── opencode/           # opencode HTTP+SSE client (pinned to v1.17.0)
│   ├── mcp/                # MCP protocol scaffold (server + client)
│   └── logging/            # Structured logging helpers
├── web/                    # React + TypeScript + Tailwind web UI (Vite)
├── configs/                # axiosd.yaml, permissions.yaml, firstboot/
├── systemd/                # axiosd.service unit (Linux installs)
├── scripts/                # scripts/axios — start/stop everything for dev
├── yocto/                  # Yocto build system (future bootable image)
└── test/                   # Integration & E2E tests
```

---

## Development

### Prerequisites

- Go 1.25+
- Node.js 20+
- An API key for at least one provider (see [Providers](#providers)) — or a local
  [Ollama](https://ollama.com) install for fully offline use
- Optional: the [opencode](https://opencode.ai) binary (v1.17.0) for the background
  coding agent

### Quick start

```bash
git clone https://github.com/Emre-Diricanli/AxiOS.git
cd AxiOS

# Configure any provider — no favorites:
export OPENAI_API_KEY=sk-...          # or ANTHROPIC_API_KEY=..., GROQ_API_KEY=...,
                                      # OPENROUTER_API_KEY=..., MISTRAL_API_KEY=..., ...

# Everything at once (MCP servers + daemon + web UI):
./scripts/axios start

# ... or piece by piece:
make dev-mcp        # MCP servers on /tmp/axios-mcp sockets
make dev-axiosd     # daemon on 127.0.0.1:3000 (uses configs/axiosd.yaml)
make dev-web        # web UI on http://localhost:5173 (proxies to the daemon)
```

### Build & test

```bash
make build       # all Go binaries into bin/
make web         # web UI production build
make vet         # go vet ./...
make test        # go test ./... -race (same as CI)
```

CI (`.github/workflows/ci.yml`) runs `go vet`, `go build`, and `go test ./... -race`
on every push and PR, plus `npm ci && npm run build` for the web UI.

### Deployment status

AxiOS is developed and run on **macOS today** (the daemon, MCP servers, web UI, and
opencode integration all work as local processes). The **Linux/Yocto bootable image**
— flash to USB, boot any x86-64 machine — is the long-term target; the `systemd/`
unit and `yocto/` layer are groundwork for it.

---

## Roadmap

| Phase | Goal | Status |
|-------|------|--------|
| **Phase 1: Model-agnostic core** | Provider layer, wired permission enforcement + approval flow, encrypted credentials, supervised opencode agent, tests + CI | Done |
| **Phase 2: Hardening & breadth** | HTTP authn/authz on REST endpoints, remaining MCP servers (docker, gpu, media, network, git, ollama), models.dev metadata cache, credential rotation | Planned |
| **Phase 3: The OS** | Linux/Yocto bootable image, first-boot wizard, OTA updates, keychain/TPM key storage | Planned |
| **Phase 4: Community** | Open source ecosystem, plugins, ARM64 | Planned |

---

## License

MIT
