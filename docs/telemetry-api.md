# Host Telemetry — API Contract

This document is the complete contract for AxiOS host telemetry: the
`axios-telemetry` agent (`cmd/axios-telemetry`), the daemon-side aggregator
(`internal/axiosd/host_telemetry.go`), and the host-config fields that wire
them together (`telemetry_port`, `telemetry_token` on `/api/hosts`). The
Hosts / system-stats UI can be built from this document alone.

## Model in one paragraph

Each remote Ollama host can run a small **telemetry agent**
(`axios-telemetry`) that exposes live hardware stats over HTTP. The agent
listens on a configured address (default `127.0.0.1:3001`) and guards every
route with a **per-host bearer token** (minimum 32 characters, compared as
SHA-256 digests in constant time). axiosd, when asked for host stats, fans
out concurrent probes: Ollama’s own `/api/version` and `/api/ps` on the
host’s Ollama port, plus — for remote hosts — the agent’s
`/api/system/stats` on the host’s `telemetry_port` using that host’s
`telemetry_token`. The **local** host never calls the agent; the daemon
gathers system stats in-process. Tokens are never returned in host list
JSON (only `has_telemetry_token: true|false`); they are stored encrypted
under `$AXIOS_DATA_DIR/hosts.json` in a separate `telemetry_tokens` map.

**Auth (axiosd surface):** `GET /api/hosts/stats` sits behind the standard
admin session middleware from `docs/auth-api.md` (session cookie or
`Authorization: Bearer axsk_...`). The agent’s own token is **not** the
admin token — it is a separate secret shared only between axiosd and that
host’s agent.

---

## axios-telemetry agent

Binary: `bin/axios-telemetry` (built from `cmd/axios-telemetry`).

### Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `-listen` | `127.0.0.1:3001` | Bind address for the telemetry HTTP API |
| `-token-file` | *(required)* | Path to a file whose contents are the bearer token (trimmed); must be ≥ 32 characters |

Example:

```bash
printf '%s\n' 'super-secret-telemetry-token-32chars-min' > /etc/axios/telemetry.token
chmod 600 /etc/axios/telemetry.token
axios-telemetry -listen 0.0.0.0:3001 -token-file /etc/axios/telemetry.token
```

The process logs `AxiOS telemetry listening on …` and serves until killed.
`ReadHeaderTimeout` is 5s; `IdleTimeout` is 30s.

### Auth scheme (agent)

Every registered route is wrapped in `authenticate` middleware:

1. Read `Authorization` header.
2. Strip a single `Bearer ` prefix (literal; remaining string is the token).
3. Compare `SHA-256(provided)` to `SHA-256(configured token)` with
   `subtle.ConstantTimeCompare`.
4. On mismatch (or missing header): `401 unauthorized` plus
   `WWW-Authenticate: Bearer`.

There is no cookie path, no admin token, and no public route. Wrong HTTP
method on a known path returns `405 method not allowed` (after auth
succeeds).

```bash
# Health check
curl -sS -H "Authorization: Bearer $TELEMETRY_TOKEN" \
  http://127.0.0.1:3001/api/health

# Hardware stats
curl -sS -H "Authorization: Bearer $TELEMETRY_TOKEN" \
  http://127.0.0.1:3001/api/system/stats
```

---

## Agent endpoints

### GET /api/health  (protected)

Liveness probe. No body.

Request:

```bash
curl -sS -H "Authorization: Bearer <token>" http://HOST:PORT/api/health
```

Responses:

| Status | Body | Meaning |
| --- | --- | --- |
| 200 | `{"status":"ok"}` | Agent up and token accepted |
| 401 | `unauthorized` (plain text) | Missing/wrong bearer token |
| 405 | `method not allowed` | Non-GET |

### GET /api/system/stats  (protected)

Live host hardware snapshot from `axiosd.GatherSystemStats()` (same shape the
daemon uses for the local host).

Request:

```bash
curl -sS -H "Authorization: Bearer <token>" \
  http://HOST:PORT/api/system/stats
```

Success `200` example (illustrative; values are host-dependent):

```json
{
  "hostname": "jetson-nano",
  "os": "linux",
  "arch": "arm64",
  "kernel": "5.15.0-tegra",
  "uptime": "3d 4h",
  "cpu": {
    "model": "ARMv8 Processor rev 1 (v8l)",
    "cores": 4,
    "threads": 4,
    "usage_percent": 12.5
  },
  "memory": {
    "total_bytes": 3964588032,
    "used_bytes": 2100000000,
    "available_bytes": 1800000000,
    "usage_percent": 53.0
  },
  "disk": [
    {
      "mount": "/",
      "device": "/dev/mmcblk0p1",
      "total_bytes": 59055800320,
      "used_bytes": 20000000000,
      "available_bytes": 36000000000,
      "usage_percent": 35.7
    }
  ],
  "gpu": [
    {
      "index": 0,
      "name": "Orin",
      "utilization_percent": 10.0,
      "memory_total_bytes": 8589934592,
      "memory_used_bytes": 1073741824,
      "memory_usage_percent": 12.5,
      "temperature_c": 48.0
    }
  ],
  "network": {
    "hostname": "jetson-nano",
    "interfaces": [
      { "name": "eth0", "ip": "192.168.1.40", "status": "up" }
    ]
  }
}
```

Field notes:

- `gpu` is populated only on Linux when `nvidia-smi` is available; otherwise
  it may be `null` or empty.
- Disk and interface lists are best-effort snapshots of the machine the
  agent runs on.

Responses:

| Status | Body | Meaning |
| --- | --- | --- |
| 200 | `SystemStats` JSON (above) | Token accepted; stats gathered |
| 401 | `unauthorized` (plain text) | Missing/wrong bearer token |
| 405 | `method not allowed` | Non-GET |
| 500 | error text | `GatherSystemStats` failed |

---

## Daemon aggregation

### GET /api/hosts/stats  (axiosd, admin-protected)

Returns a single `HostTelemetry` object for one registered Ollama host.

Query:

| Param | Required | Meaning |
| --- | --- | --- |
| `id` | no | Host id (slug). If omitted, uses the **active** host |

```bash
# Active host
curl -sS -H "Authorization: Bearer axsk_..." \
  http://127.0.0.1:3000/api/hosts/stats

# Specific host
curl -sS -H "Authorization: Bearer axsk_..." \
  'http://127.0.0.1:3000/api/hosts/stats?id=jetson-nano'
```

Responses:

| Status | Body | Meaning |
| --- | --- | --- |
| 200 | `HostTelemetry` JSON | Aggregate snapshot |
| 404 | `{"error":"host not found"}` | Unknown `id` or no active host |
| 405 | `{"error":"method not allowed"}` | Non-GET |
| 503 | `{"error":"host management not initialized"}` | Host store not wired |

### HostTelemetry shape

```json
{
  "host": {
    "id": "jetson-nano",
    "name": "Jetson Nano",
    "host": "192.168.1.40",
    "port": 11434,
    "telemetry_port": 3001,
    "status": "online",
    "models": ["llama3.2:latest"],
    "active": true,
    "gpu_info": "",
    "has_telemetry_token": true
  },
  "source": "agent",
  "system": { "...": "SystemStats when available" },
  "ollama_version": "0.6.5",
  "running_models": [
    {
      "name": "llama3.2:latest",
      "size_bytes": 2019393189,
      "vram_bytes": 2019393189
    }
  ],
  "latency_ms": 42,
  "message": ""
}
```

| Field | Meaning |
| --- | --- |
| `host` | Snapshot of the `OllamaHost` record (token never included) |
| `source` | How system stats were obtained: `"local"`, `"agent"`, or `"ollama"` (fallback when agent stats are missing) |
| `system` | `SystemStats` when available; omitted/null when agent poll failed |
| `ollama_version` | From Ollama `GET /api/version` when reachable |
| `running_models` | From Ollama `GET /api/ps` (`name`, `size` → `size_bytes`, `size_vram` → `vram_bytes`); always a JSON array (possibly empty) |
| `latency_ms` | Wall time for the parallel fan-out (milliseconds) |
| `message` | Human-readable reason when remote hardware telemetry is unavailable |

### Probe plan (`collectHostTelemetry`)

All HTTP probes use a client timeout of **2.5s** and run in parallel:

| Target | URL | Auth | When |
| --- | --- | --- | --- |
| Ollama version | `http://{host}:{port}/api/version` | none | always |
| Ollama processes | `http://{host}:{port}/api/ps` | none | always |
| Agent system stats | `http://{host}:{telemetry_port}/api/system/stats` | `Authorization: Bearer {telemetry_token}` | remote hosts only (`id != "local"`) |

Port selection for the agent:

1. Use `host.telemetry_port` when `> 0`.
2. Otherwise fall back to **3000** (`defaultTelemetryPort` in
   `host_telemetry.go`). New hosts created via the API default
   `telemetry_port` to `3000` in the store.

**Local host (`id == "local"`):**

- `source` is forced to `"local"`.
- System stats come from in-process `gatherSystemStats()` — the agent is
  **not** contacted.
- Ollama version / running models still come from the local Ollama port when
  reachable.

**Remote host:**

- On successful agent response: `source = "agent"`, `system` filled.
- On agent failure:
  - If `telemetry_token` is empty:
    `message = "A telemetry token is required for AxiOS on port {port}"`
  - Else:
    `message = "Authenticated hardware telemetry is unavailable on port {port}"`
  - `source` remains `"ollama"`; Ollama fields may still be populated.

Ollama probe failures are silent (fields stay empty); only the agent path
sets `message`.

---

## Host-config fields

Stored on each `OllamaHost` and exposed through `/api/hosts`.

| JSON field | Type | API-visible | Notes |
| --- | --- | --- | --- |
| `telemetry_port` | int | yes | Agent listen port on that machine. Default **3000** for newly added hosts. Must be in `1..65535` when set via store helpers. |
| `telemetry_token` | string | **write-only** (`json:"-"`) | Bearer secret for the agent. Never serialized in list/get responses. Empty clears. Non-empty values must be **≥ 32** characters. |
| `has_telemetry_token` | bool | yes | `true` when a non-empty token is configured; safe for UI badges. |

Persistence (`$AXIOS_DATA_DIR/hosts.json`, mode 0600):

- Host records are written **without** plaintext tokens.
- Tokens live in a sibling map `telemetry_tokens: { "<host-id>": "axsec1:..." }`,
  encrypted with the secrets store (`pkg/secrets`). Saving a token without an
  encrypted secrets store fails closed.

### POST /api/hosts  (create)

Optional telemetry fields on create:

```json
{
  "name": "Jetson Nano",
  "host": "192.168.1.40",
  "port": 11434,
  "telemetry_port": 3001,
  "telemetry_token": "super-secret-telemetry-token-32chars-min"
}
```

- `telemetry_port`: if `> 0`, applied after create; if omitted/`0`, the store
  default `3000` remains. Values outside `0..65535` → `400`.
- `telemetry_token`: if non-empty, validated (≥ 32 chars) and stored; omitted
  or `""` leaves the host without a token.

Response `201` is the created host object (includes `telemetry_port` and
`has_telemetry_token`, never the raw token).

### PATCH /api/hosts  (update telemetry config)

```json
{
  "id": "jetson-nano",
  "telemetry_port": 3001,
  "telemetry_token": "super-secret-telemetry-token-32chars-min"
}
```

| Field | Behavior |
| --- | --- |
| `id` | Required host slug |
| `telemetry_port` | Always applied via `SetTelemetryPort` (must be `1..65535`) |
| `telemetry_token` | Optional pointer: **omit** to leave token unchanged; send `""` to clear; send a new string (≥ 32 chars) to replace |

Success:

```json
{ "ok": true, "host": [ /* full host list */ ] }
```

Errors: `400` for bad JSON / validation / unknown id; `500` if persist fails;
`503` if host management is not initialized.

### Recommended setup sequence

1. On the remote machine, generate a long random token (≥ 32 chars) and write
   it to a root-only file.
2. Start `axios-telemetry -listen 0.0.0.0:3001 -token-file …` (or a systemd
   unit) so the agent is reachable from the axiosd host.
3. In axiosd, `POST /api/hosts` (or `PATCH`) with matching `telemetry_port`
   and `telemetry_token`.
4. Confirm with `GET /api/hosts/stats?id=<id>` — expect
   `"source":"agent"` and a populated `system` object when network and token
   match.

---

## Security notes

- Treat each host’s telemetry token like a machine credential: unique per
  host, stored only as `axsec1:` ciphertext on disk, never logged.
- Prefer binding the agent to a Tailscale/private interface rather than the
  public internet; axiosd reaches it over plain HTTP today.
- Agent auth is independent of axiosd’s admin `axsk_…` token. Compromising
  one does not grant the other.
- `GET /api/hosts/stats` remains admin-only until a machine-auth path is
  registered (see “Machine endpoints” in `docs/auth-api.md`).
