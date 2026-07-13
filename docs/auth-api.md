# axiosd Admin Authentication — API Contract

This document is the complete contract between the axiosd auth layer
(`internal/axiosd/auth.go`, `auth_api.go`) and its clients: the React SPA,
OpenAI-compatible API consumers, and future machine endpoints. The login UI
can be built from this document alone.

## Model in one paragraph

axiosd uses a **Jupyter-style admin token**. On first start the daemon
generates `axsk_` + 43 chars of base64url (32 random bytes), prints it ONCE
to the daemon log inside a framed banner, and stores only its SHA-256 hash in
`$AXIOS_DATA_DIR/auth.json` (mode 0600). The browser exchanges the token for
a stateless **HMAC-signed session cookie** at `POST /api/auth/login`.
Non-browser clients skip cookies entirely and send the token as
`Authorization: Bearer axsk_...` on every request — the admin token doubles
as the API key for `/v1/*` and curl. The token cannot be recovered; it can
only be reset with `axiosd --reset-auth`, which also invalidates every
outstanding session.

Auth is **enabled by default**. `server.auth.enabled: false` in
`configs/axiosd.yaml` disables enforcement entirely (middleware passes
everything through) — the WebSocket origin check stays active regardless,
because cross-site WebSocket hijacking protection is not an auth feature.

---

## Endpoints

### POST /api/auth/login  (public)

Request:

```json
{ "token": "axsk_..." }
```

Responses:

| Status | Body | Meaning |
| --- | --- | --- |
| 200 | `{"ok":true}` + `Set-Cookie: axios_session=...` | Token accepted, session issued |
| 400 | `{"error":"invalid JSON"}` | Malformed body |
| 401 | `{"error":"invalid token"}` | Wrong token (constant-time hash compare) |
| 403 | `{"error":"origin not allowed"}` | Cross-origin login attempt (login CSRF) |
| 405 | `{"error":"POST required"}` | Wrong method |
| 429 | `{"error":"too many attempts"}` | Rate limited (see below) |

Rate limit: **5 failed attempts per minute per remote IP** (sliding window).
Once exhausted, every further attempt from that IP — even with the correct
token — returns 429 until the window slides. Only the TCP `RemoteAddr` is
consulted; `X-Forwarded-For` is deliberately ignored. A successful login
clears the IP's failure count.

### POST /api/auth/logout  (protected)

No body. Responds `204 No Content` and expires the cookie (`Max-Age=-1`).
Requires auth like any other `/api` route (an anonymous call gets 401 —
harmless). Because sessions are stateless, logout only clears the browser's
cookie; revoking **all** sessions everywhere is `axiosd --reset-auth`.

### GET /api/auth/status  (public, always)

```json
{ "auth_required": true, "authenticated": false }
```

- `auth_required` — whether the middleware is enforcing (config
  `server.auth.enabled`).
- `authenticated` — whether **this** request carried a valid session cookie
  or bearer token.

This endpoint never returns anything else and never 401s.

---

## How the SPA should gate (Codex: build this)

1. On app boot, `GET /api/auth/status`.
2. `auth_required == false` → proceed straight to the app.
3. `auth_required == true && authenticated == true` → proceed (cookie is
   already present and valid).
4. `auth_required == true && authenticated == false` → render the **login
   screen**: a single field for the `axsk_...` token, POST it to
   `/api/auth/login`, then re-check status (or just proceed on 200). Show
   "invalid token" on 401 and "too many attempts — wait a minute" on 429.
5. Any API response with **401** at runtime (e.g. session expired after 7
   days, or `--reset-auth` was run) → drop back to the login screen. The
   daemon never redirects; it always answers `401 {"error":"unauthorized"}`
   as JSON.
6. Logout button → `POST /api/auth/logout`, then show the login screen.

The SPA shell itself (index.html, JS/CSS assets, any non-`/api`/`/ws`/`/v1`
path) is always served without auth, so the login screen can render.

### Cookie semantics

- Name `axios_session`, value `v1.<epoch>.<expiresUnix>.<nonce>.<hmac>`
  (opaque to clients — do not parse).
- `HttpOnly` (invisible to JS), `SameSite=Lax`, `Path=/`.
- `Secure` is set automatically when the request arrived over TLS
  (`r.TLS != nil`) or through a TLS-terminating proxy
  (`X-Forwarded-Proto: https`, e.g. `tailscale serve`).
- Lifetime: `server.auth.session_ttl_hours` (default 168 h = 7 days).
- The browser attaches it automatically to `fetch` (same-origin) and to
  WebSocket upgrades. **No Authorization header handling is needed in the
  SPA.** With the Vite dev server (`:5173` proxying to `:3000`), the cookie
  flows through the proxy unchanged.

---

## Route protection map

A single middleware wraps the entire mux, so **every route is protected by
default** — including routes added in the future.

| Class | Paths | Credential |
| --- | --- | --- |
| Public | Anything **not** under `/api`, `/ws`, `/v1` (SPA shell + assets) | none |
| Public | `/api/auth/login`, `/api/auth/status` | none |
| Admin | **Everything else** under `/api/`, `/ws`, `/ws/`, `/v1/` | session cookie **or** `Authorization: Bearer axsk_...` |
| Machine | *(none yet — see "Machine endpoints")* | reserved |

Unauthenticated protected requests → `401 {"error":"unauthorized"}` (JSON,
never a redirect). Paths are cleaned before classification, so `//api/x` or
`/foo/../api/x` cannot dodge the check.

## Origin / CSRF rules

State-changing HTTP requests (anything except GET/HEAD/OPTIONS) that carry
an `Origin` header are checked **before** auth; a mismatch returns
`403 {"error":"origin not allowed"}` even with valid credentials. WebSocket
upgrades apply the identical policy via `Upgrader.CheckOrigin`. An origin is
accepted when **any** of these holds:

1. No `Origin` header at all (curl, SDKs, native apps — they still need the
   bearer token or cookie).
2. Origin host equals the request `Host`, ignoring ports.
3. Both origin host and request host are loopback
   (`localhost`/`127.0.0.1`/`::1`) — this keeps the Vite dev server on
   `:5173` talking to axiosd on `:3000`.
4. The origin is listed in config `server.auth.allowed_origins`
   (exact origin match, case-insensitive, trailing `/` ignored). Needed only
   behind a reverse proxy whose public origin differs from the Host axiosd
   sees.

`null` and malformed origins are always rejected.

## WebSockets (`/ws`, `/ws/terminal`)

- Upgrades require auth like any `/api` route: the **cookie rides the
  upgrade request** automatically for browsers. An unauthenticated upgrade
  attempt is answered `401` before the handshake.
- Non-browser WS clients send the admin token as a header on the upgrade
  request: `Authorization: Bearer axsk_...`.
- `CheckOrigin` enforces the origin rules above (this check runs even when
  auth is disabled).

## API clients / OpenAI-compatible surface (`/v1/*`)

Use the admin token as the API key — standard OpenAI-client configuration:

```bash
curl -H "Authorization: Bearer axsk_..." http://127.0.0.1:3000/v1/models

# Open WebUI / LangChain / SDKs: base URL http://127.0.0.1:3000/v1,
# API key axsk_...
```

The `Bearer` scheme is case-insensitive; the token is compared by SHA-256
hash in constant time.

---

## Machine endpoints (hook for the per-host telemetry token scheme)

`internal/axiosd/auth.go` defines:

```go
var machineAuthEndpoints = map[string]authClass{}
```

It ships **empty**, so e.g. `/api/hosts/stats` currently requires admin
credentials like everything else. When the per-host telemetry token scheme
(owned by Codex) lands:

1. Add the exact, cleaned path to the table, mapped to `authMachine`:
   `machineAuthEndpoints["/api/telemetry/ingest"] = authMachine`.
2. Implement the machine-token verifier and handle the `authMachine` case in
   `AuthManager.Middleware` (today it falls through to admin credentials —
   fail closed until the verifier exists).

Paths in this table are matched after `path.Clean`, before the public
allowlist and the `/api`-prefix rule, so a table entry fully controls its
path's auth class (including making it public via `authPublic`, though
that should be rare and deliberate).

## Token lifecycle / --reset-auth

- First start: token generated, printed once to the daemon log (stderr)
  inside a `====` framed banner, hash stored in
  `$AXIOS_DATA_DIR/auth.json` (0600) together with the HMAC session key and
  `epoch: 1`.
- `axiosd --reset-auth`: regenerates token + session key, bumps `epoch`
  (which invalidates **every** outstanding session cookie), prints the new
  banner, exits 0. Run it when the token is lost or suspected leaked.
- A corrupt `auth.json` makes the daemon refuse to start (it never silently
  regenerates credentials); `--reset-auth` is the recovery path.
- The plaintext token is never logged outside the generation/reset banner.

## Config reference (`configs/axiosd.yaml`)

```yaml
server:
  listen: "127.0.0.1:3000"   # keep loopback; expose via tailscale serve / reverse proxy
  auth:
    enabled: true            # default true even when the key is omitted
    session_ttl_hours: 168   # 7 days
    allowed_origins: []      # extra browser origins for reverse-proxy setups
```

TLS is intentionally out of scope for axiosd: front it with
`tailscale serve` or a reverse proxy and keep `listen` on `127.0.0.1`.
