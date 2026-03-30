# AxiOS

AI-native operating system. Claude is the primary interface, with local model (Ollama) fallback.

## Project structure

- `cmd/` — Go binary entry points (claused daemon + MCP servers)
- `internal/` — Private Go packages (claused internals)
- `pkg/` — Shared Go packages (MCP protocol, permissions, logging)
- `web/` — React + TypeScript + Tailwind web UI (Vite)
- `yocto/` — Yocto build system and meta-axios layer
- `configs/` — Runtime config files (permissions.yaml, claused.yaml)
- `systemd/` — Systemd service unit files
- `scripts/` — Dev and build scripts
- `deploy/` — Docker-based local dev environment

## Build

- Go workspace: `go.work` at root
- Web UI: `cd web && npm install && npm run dev`
- Full image: `scripts/build-image.sh`

## Conventions

- Go: standard project layout, `cmd/` for entry points, `internal/` for private code
- MCP servers: each is a Go binary in `cmd/axios-*`, uses `pkg/mcp` scaffold
- MCP transport: Unix sockets at `/run/axios/mcp/*.sock`
- Permissions: enforced by claused, defined in `configs/permissions.yaml`
- Tests: unit tests beside source (`_test.go`), integration tests in `test/`
