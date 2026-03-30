# AxiOS

**An AI-native operating system where Claude is the primary interface — not an app, but the OS itself.**

AxiOS (from Greek *axios*, meaning "worthy") is a hardware-agnostic creator workstation OS that boots any x86-64 machine into an intelligent environment. Claude has deep, tiered hardware access through MCP servers, and serves as the primary interface for all creative and system workflows.

> *Your hardware is worthy. Your creative work is worthy. This machine deserves intelligence.*

---

## What is this?

AxiOS is a standalone Linux-based operating system you flash onto a USB, boot on any x86-64 machine, and immediately start talking to Claude. No setup beyond plugging in your API key. Claude can manage your files, spin up Docker containers, monitor your GPU, run media encodes, and orchestrate your entire creative workflow — all through natural conversation.

If your hardware has a capable GPU, local models (via Ollama) handle quick tasks, provide offline capability, and keep things running when the cloud API is unreachable.

### Core Principles

1. **Claude IS the OS** — Boot up and Claude is there. Not an app on top of another OS.
2. **Local AI too** — Local models handle quick tasks, offline operation, and privacy-sensitive queries.
3. **Hardware-agnostic** — Old Dell servers, Intel NUCs, custom builds — anything x86-64 with UEFI.
4. **Creator-first** — Pre-configured media pipelines, GPU compute, code environments, content tools.
5. **Tiered trust** — Claude has full access for safe operations, asks permission for destructive ones.
6. **Your machine, your data** — Local-first, privacy-respecting, no cloud dependency for basic tasks.

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  User Interface                 │
│         (Web UI / Custom DE / Remote)           │
├─────────────────────────────────────────────────┤
│                    claused                      │
│          (Claude Integration Daemon)            │
│     Auth · Context · Permissions · Sessions     │
├────────────────┬────────────────────────────────┤
│  AI Backends   │         MCP Servers            │
│  ┌───────────┐ │  ┌────┬──────┬─────┬────────┐  │
│  │ Claude    │ │  │ fs │docker│ gpu │system  │  │
│  │ (primary) │ │  ├────┼──────┼─────┼────────┤  │
│  ├───────────┤ │  │media│ net │ git │ ollama │  │
│  │ Ollama    │ │  └────┴──────┴─────┴────────┘  │
│  │ (local)   │ │                                │
│  └───────────┘ │                                │
├────────────────┴────────────────────────────────┤
│              Docker / containerd                │
├─────────────────────────────────────────────────┤
│           Yocto Linux (systemd)                 │
│     NVIDIA/AMD drivers · Tailscale · NM         │
├─────────────────────────────────────────────────┤
│              x86-64 Hardware (UEFI)             │
└─────────────────────────────────────────────────┘
```

**`claused`** — The core daemon (Go). Connects to the Anthropic API, routes between Claude and local models, manages MCP server lifecycle, enforces the permission model, and serves the web UI over WebSocket.

**MCP Servers** — Each server exposes a domain of system control (filesystem, Docker, GPU, networking, etc.) as tools that Claude can call. They communicate with `claused` over Unix sockets.

**Web UI** — A React + TypeScript + Tailwind application served locally. Chat-first interface with system monitoring panels, container management, terminal, and file browser.

**Yocto Image** — A minimal Linux image built with Yocto, including the kernel, GPU drivers, Docker, Tailscale, and all AxiOS components. Flash to USB and boot.

---

## Project Structure

```
AxiOS/
├── cmd/                    # Go binary entry points
│   ├── claused/            # Core daemon
│   ├── axios-fs/           # MCP: filesystem
│   ├── axios-docker/       # MCP: container management
│   ├── axios-gpu/          # MCP: GPU management
│   ├── axios-system/       # MCP: system info & control
│   ├── axios-media/        # MCP: audio/video processing
│   ├── axios-network/      # MCP: networking & Tailscale
│   ├── axios-git/          # MCP: version control
│   └── axios-ollama/       # MCP: local model management
├── internal/               # Private Go packages
│   ├── claused/            # Daemon internals (server, routing, sessions)
│   └── config/             # Configuration loading
├── pkg/                    # Shared Go packages
│   ├── mcp/                # MCP protocol implementation
│   ├── permissions/        # Tiered trust model
│   └── logging/            # Structured logging
├── web/                    # React + TypeScript web UI
│   └── src/
│       ├── components/     # Chat, System, Terminal, Files, Layout
│       ├── hooks/          # WebSocket connection
│       ├── lib/            # API helpers
│       └── types/          # Shared types
├── yocto/                  # Yocto build system
│   ├── meta-axios/         # Custom Yocto layer
│   └── kas/                # Build configurations
├── configs/                # Runtime configuration
│   ├── claused.yaml        # Daemon config
│   ├── permissions.yaml    # Permission tiers
│   └── firstboot/          # Setup wizard config
├── systemd/                # Systemd service units
├── scripts/                # Dev and build scripts
├── deploy/                 # Docker-based local dev
└── test/                   # Integration & E2E tests
```

---

## Permission Model

AxiOS uses a three-tier trust system:

| Tier | Examples | Behavior |
|------|----------|----------|
| **Trusted** | Read files, list containers, query GPU, git commit | Executes immediately |
| **Approval Required** | Delete files, format disk, reboot, firewall changes | Asks user for confirmation |
| **Prohibited** | Modify own config, export credentials, disable auth | Refused regardless |

Users can promote/demote operations between Trusted and Approval Required, but never modify Prohibited. Permissions are enforced by `claused` and the MCP servers, not by the model.

---

## Creator Workflows

AxiOS ships with pre-configured support for four creator domains, pulled as Docker containers during first boot:

- **Media Production** — FFmpeg, HandBrake, ImageMagick, DaVinci Resolve, OBS
- **Code Development** — VS Code Server, Gitea, CI/CD pipelines, dev databases
- **AI/ML Workloads** — Ollama, JupyterLab, PyTorch, ComfyUI, TensorBoard
- **Content Creation** — Ghost, Obsidian, Typst, Hugo/Astro, Minio

---

## Development

### Prerequisites

- Go 1.25+
- Node.js 22+
- Anthropic credentials (one of the following):
  - **OAuth token** from your Claude subscription — run `claude setup-token` (free with Pro/Max)
  - **API key** from [console.anthropic.com](https://console.anthropic.com) (pay per token)

### Quick Start

```bash
git clone https://github.com/Emre-Diricanli/AxiOS.git
cd AxiOS

# Option 1: Use your Claude subscription (free)
claude setup-token  # copy the token it outputs
export ANTHROPIC_OAUTH_TOKEN=sk-ant-oat01-...

# Option 2: Use an API key (paid per token)
export ANTHROPIC_API_KEY=sk-ant-api03-...

# Start the daemon
make dev-claused

# In another terminal — start the web UI
make dev-web
```

The web UI runs on `http://localhost:5173` and proxies API/WebSocket calls to `claused` on port 3000.

### Build

```bash
# Build all Go binaries
make build

# Build the web UI
make web

# Run tests
make test
```

---

## Roadmap

| Phase | Goal | Status |
|-------|------|--------|
| **Phase 1: PoC** | Bootable image, Claude connected, basic chat, bash/file access | In progress |
| **Phase 2: MVP** | MCP servers, permission model, first boot wizard, Ollama integration | Planned |
| **Phase 3: Polish** | Full web UI, OTA updates, app catalog, mobile-responsive | Planned |
| **Phase 4: Community** | Open source ecosystem, custom DE, ARM64, plugins | Planned |

---

## How is this different?

| | OpenClaw | ZimaOS | AxiOS |
|---|---|---|---|
| **Claude integration** | App layer | None | OS-native, hardware-level |
| **Local AI** | Ollama as optional | None | Ollama as fallback + quick tasks |
| **Hardware** | Mac-focused | x86-64 + Zima devices | Any x86-64 with UEFI |
| **Interface** | Node.js + messaging | Web UI for NAS | AI-first web UI + optional DE |
| **GPU access** | Via local models | Limited | Direct NVIDIA/AMD via MCP |
| **Permission model** | Full access or sandboxed | N/A | Tiered trust system |
| **Offline capable** | Partial | N/A | Yes (local model fallback) |
| **Form factor** | App on existing OS | NAS OS | Standalone OS |

---

## License

MIT
