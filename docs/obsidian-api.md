# axiosd Obsidian Vault — API Contract

This document is the complete contract between the axiosd Obsidian
integration (`internal/axiosd/obsidian_api.go`, engine in
`internal/obsidianctl/`) and the web UI. A vault settings card, a notes
browser, a note editor, and a search box can be built from this document
alone.

## Model in one paragraph

An Obsidian vault is just a **directory of `.md` files** — axiosd reads and
writes it directly on disk; the Obsidian app is never involved and does not
need to be running or even installed. The active vault path lives in
`$AXIOS_DATA_DIR/obsidian.json`, written by `PUT /api/obsidian/vault` and
re-read on every request, so switching vaults takes effect immediately in the
daemon **and** in the `axios-obsidian` MCP server (the chat model's tools)
without a restart. Until the state file exists, `obsidian.vault` in
`configs/axiosd.yaml` seeds the selection. All note paths in this API are
**vault-relative** (`Work/AxiOS/notes.md`); absolute paths, `..` segments,
backslashes and dot-prefixed names (`.obsidian/`, `.trash/`, …) are rejected,
and `.md` is appended automatically when the caller omits it.

**Auth:** every endpoint below sits behind the standard admin session
middleware from `docs/auth-api.md` (session cookie or
`Authorization: Bearer axsk_...`). Unauthenticated requests get
`401 {"error":"unauthorized"}`. Nothing here is public.

---

## The unconfigured state (and the 409 flow)

"No vault configured" is a **normal state**, not an error condition:

- `GET /api/obsidian/status` always answers `200` and reports
  `"configured": false`.
- **Every other endpoint** (notes, note, search) answers
  `409 {"error":"no vault configured"}`.
- A configured-but-broken vault (directory moved/deleted) also answers `409`,
  with `{"error":"vault unavailable: ..."}`.

**UI rule: any 409 from `/api/obsidian/*` → route the user to the vault
settings card.** After a successful `PUT /api/obsidian/vault`, retry.

## Suggested Settings card (Codex: build this)

1. On opening Settings (or the Obsidian page), `GET /api/obsidian/status`.
2. `configured == false` → show a **vault path input** (absolute directory
   path, e.g. `/Users/emrediricanli/Documents/Obsidian`) with a "Connect
   vault" button that `PUT`s `/api/obsidian/vault`.
3. `configured == true` → show the status display: vault name, path, note
   and folder counts, human-readable size — plus a "Change vault" affordance
   that reuses the same input.
4. `looks_like_vault == false` → show a non-blocking warning ("This folder
   has no `.obsidian/` marker — it works, but double-check the path"). Any
   directory of Markdown files is accepted.
5. `PUT` failures are `400` with a human-readable `error` (relative path,
   nonexistent directory, not a directory) — render it inline at the input.

---

## Endpoints

All request/response bodies are JSON. Errors are always
`{"error":"message"}` with an appropriate status. Every handler enforces its
method (wrong method → `405`).

### GET /api/obsidian/status

Never 409s — the UI gates on this endpoint.

Unconfigured:

```json
{ "configured": false }
```

Configured:

```json
{
  "configured": true,
  "vault_path": "/Users/emrediricanli/Documents/Obsidian",
  "name": "Obsidian",
  "looks_like_vault": true,
  "notes": 128,
  "folders": 14,
  "size_bytes": 512345
}
```

- `name` — `basename` of the vault directory.
- `looks_like_vault` — whether the directory contains `.obsidian/`.
- `notes` / `folders` / `size_bytes` — visible `.md` notes, visible folders,
  and the summed size of the notes. Hidden (dot-prefixed) entries are never
  counted.
- Configured but unusable (directory vanished): `configured: true` +
  `vault_path` + `looks_like_vault: false` + an `error` field, **stats
  omitted** — show the settings card with the error.

### PUT /api/obsidian/vault

Request:

```json
{ "path": "/Users/emrediricanli/Documents/Obsidian" }
```

Responses:

| Status | Body | Meaning |
| --- | --- | --- |
| 200 | the full status payload (above) | Vault validated and persisted |
| 400 | `{"error":"..."}` | Missing/relative/nonexistent path, or not a directory |
| 405 | `{"error":"PUT required"}` | Wrong method |

On 200 the choice is persisted to `$AXIOS_DATA_DIR/obsidian.json` and is live
everywhere immediately (REST + chat tools). The config seed never overrides
it again.

### GET /api/obsidian/notes?folder=&recursive=true

Lists notes **and folders** under a vault-relative folder (omit `folder` for
the vault root) so the UI can tree-browse. `recursive=true` descends into
subfolders; anything else is non-recursive. Only `.md` files and folders
appear; hidden entries never do.

```json
{
  "entries": [
    { "path": "Work",         "name": "Work",    "is_folder": true,  "size": 0,   "modified": "2026-07-10T09:30:00Z" },
    { "path": "Work/plan.md", "name": "plan.md", "is_folder": false, "size": 512, "modified": "2026-07-11T18:02:41Z" }
  ]
}
```

Entries are sorted by `path` (byte order — uppercase before lowercase).
`modified` is RFC 3339 UTC. For a lazy tree, request non-recursively per
folder using the entry's `path` as the next `folder` param.

Errors: `400` invalid folder path, `404` folder does not exist, `409`
unconfigured.

### GET /api/obsidian/note?path=X

Reads one note. `path` is vault-relative; `.md` optional
(`Work/plan` ≡ `Work/plan.md`).

```json
{
  "path": "Work/plan.md",
  "content": "---\ntags:\n  - work\n---\n# Plan\nShip the vault integration. #q3\n",
  "frontmatter": { "tags": ["work"] },
  "tags": ["q3", "work"],
  "size": 512,
  "modified": "2026-07-11T18:02:41Z"
}
```

- `content` is the **raw file**, including the frontmatter block — feed it
  straight into the editor.
- `frontmatter` is the parsed leading `---` YAML block; **absent/`null`**
  when the note has none or the block is malformed (the note is still
  returned — never fail a read over bad YAML).
- `tags` merges frontmatter `tags` (string or list) with inline `#tag`
  tokens from the body: deduplicated case-insensitively, `#` stripped,
  sorted. Always an array (possibly empty).

Errors: `400` missing `path` param / invalid path, `404` no such note, `409`
unconfigured.

### PUT /api/obsidian/note

Request:

```json
{ "path": "Journal/2026-07-15", "content": "# Today\n...", "overwrite": true }
```

- `overwrite` **defaults to `true`** — the editor's save button can omit it.
  Send `overwrite: false` only for "create new note" flows where clobbering
  an existing note must fail.
- Parent folders are created automatically inside the vault.

Responses: `200 {"ok":true}`; `400` missing path / invalid path / bad JSON;
`409` note exists and `overwrite` was `false` (or vault unconfigured — tell
them apart by the `error` text, or avoid the ambiguity by checking status
first); `405` wrong method (only GET/PUT/DELETE exist on this route).

### DELETE /api/obsidian/note?path=X

Deletes a single note. Folders are never deleted through this API.

Responses: `204` (no body) on success; `400` missing/invalid path (including
folder targets); `404` no such note; `409` unconfigured.

### GET /api/obsidian/search?q=&tag=&limit=

Case-insensitive **substring** search over note filenames and content. At
least one of `q` and `tag` is required (`400` otherwise):

- `q` — the substring. A filename-only match still counts as a hit.
- `tag` — restrict hits to notes carrying the tag (frontmatter or inline;
  leading `#` optional; case-insensitive). With an empty `q`, `tag` alone
  lists all tagged notes.
- `limit` — max hits, clamped to 1–100, default 20.

Notes larger than 1 MiB are skipped; hidden directories are never searched.

```json
{
  "hits": [
    {
      "path": "Work/plan.md",
      "name": "plan.md",
      "snippet": "…roughly 160 characters centered on the first match…",
      "modified": "2026-07-11T18:02:41Z"
    }
  ]
}
```

`snippet` is ≈160 characters centered on the first content match
(filename-only matches show the start of the note), whitespace-collapsed to
a single line — render it as-is under the note name. `hits` is always an
array (possibly empty).

---

## Status code summary

| Status | When |
| --- | --- |
| 200 | Success (status, vault, notes, note read/write, search) |
| 204 | Note deleted |
| 400 | Invalid vault path (PUT vault), invalid/missing note path, bad JSON, missing `q`/`tag` |
| 401 | Not authenticated (see `docs/auth-api.md`) |
| 404 | Note or folder does not exist |
| 405 | Wrong HTTP method |
| 409 | No vault configured / vault unavailable, or `overwrite:false` on an existing note |
| 500 | Unexpected filesystem failure |

## Chat-side counterpart (context, not for the UI)

The same vault powers the `axios-obsidian` MCP server the chat model uses:
`search_notes` / `read_note` / `list_notes` / `vault_info` are trusted;
`write_note` / `append_note` / `delete_note` require user approval through
the standard WebSocket approval flow. Setting the vault in the Settings card
is what makes those tools work — the MCP server reads the same
`obsidian.json` state file per call, no restart needed.
