# Chaind — How It Works & What Was Fixed

A plain-language guide. No jargon without an explanation.

---

## 1. What is this project?

**Chaind** is a small program that sits on your own computer and collects your
chat messages (WhatsApp, Telegram, Matrix) into one local database. Then your
own scripts or AI agents can read, search, and reply to those messages through
a safe local door (called IPC).

Think of it like this:

```
Your chats (WhatsApp / Telegram / Matrix)
        │  adapters listen live, like ears
        ▼
Local SQLite database (messages.db) — your notebook
        │  IPC server guards the door with tokens
        ▼
CLI commands (send, read, search, watch...) — your hands
```

No cloud. No company server. Everything stays on your machine.

---

## 2. How it works, step by step

### Step 1 — You start the daemon

```bash
./chaind daemon start
```

The **daemon** is a background worker that never sleeps. It does four jobs:

1. Opens the local database (`~/.local/share/chaind/messages.db`).
2. Starts a writer that saves messages safely, one batch at a time.
3. Opens a local socket (`~/.config/chaind/chaind.sock`) — the "door".
4. Connects the chat adapters (WhatsApp QR scan, Telegram token, Matrix token).

### Step 2 — Adapters listen to chats

An **adapter** is a translator for one chat app:

- **WhatsApp adapter** logs in as a linked device (QR code) and hears new
  messages as they arrive.
- **Telegram adapter** logs in with your bot token via MTProto.
- **Matrix adapter** syncs with your homeserver.

Every message they hear is converted into one **common format**
(`schema.Message`: who, which room, what text, when) and saved to the database.

### Step 3 — Messages are stored

The **store** (`internal/store/`) writes each message with a unique ID (ULID —
a sortable unique stamp). A background writer batches inserts so the database
never gets corrupted by two writers at once.

### Step 4 — You get a token (your key)

```bash
export CHAIND_TOKEN=$(./chaind token issue --role owner)
```

The **token** is a secret key. Every request to the daemon must show it.
Tokens have **tiers** (owner = can do everything, agent = limited rooms,
readonly = look only) and optional **room scopes** (which chats it may touch).

### Step 5 — You use CLI commands through the door

```bash
./chaind read                    # last 50 messages
./chaind search -q "hello"       # full-text search
./chaind watch                   # live stream of new messages
./chaind send --platform matrix --room "!abc:example.com" --text "hi"
```

Each CLI command is just an HTTP request sent over the local Unix socket,
carrying your token in the `Authorization` header. The daemon checks the
token, does the job, and answers in JSON.

---

## 3. What was broken — and what I fixed (11 commits)

### Fix 1 — Tokens were stored as plain text, expiry was fake
**Commit `717cb89`**

- **Before:** Your secret token sat in the database exactly as-is. Anyone who
  copied the database file owned every token. The `--expires` flag was
  ignored, and expired tokens still worked forever.
- **After:** Only a scrambled fingerprint (SHA-256 hash) is stored. The real
  secret is shown once and never saved. `--expires 30d` is honored, and the
  daemon rejects expired tokens.
- **Files:** `internal/store/repository.go`, `cmd/cli_token.go`,
  `internal/ipc/socket.go`

### Fix 2 — Viewing/revoking tokens could crash the program
**Commit `5379264`**

- **Before:** `token list` and `token revoke` sliced strings without checking
  length. A long or short input could panic (crash).
- **After:** Safe prefix matching. You can revoke with a short prefix, the
  full fingerprint, or the original secret — no crash possible.
- **Files:** `cmd/cli_token.go`

### Fix 3 — Error messages broke the JSON and leaked internals
**Commit `b10471a`**

- **Before:** Internal errors (file paths, database details) were pasted raw
  into responses, sometimes producing invalid JSON.
- **After:** One safe helper sends clean `{"error": "..."}` replies; full
  details stay in server logs. Search is capped (1–100 results, 200-char
  queries) so one bad request can't dump the whole database.
- **Files:** `internal/ipc/socket.go`

### Fix 4 — Privacy redaction was completely dead
**Commit `52dac92`**

- **Before:** Two bugs: (a) the code looked up the token under the wrong name,
  so redaction never ran; (b) the token text was treated as a custom search
  pattern, which could freeze the server (ReDoS).
- **After:** Correct token lookup, fixed safe detectors only
  (email / phone / PAN), applied to history, search, **and** the live stream.
- **Files:** `internal/ipc/pii.go`, `internal/ipc/socket.go`

### Fix 5 — Reply/react/delete ignored room permissions
**Commit `8d0d4a4`**

- **Before:** Permission checks only looked at a `room` field. Reply, react,
  and delete send a message *ID* instead, so limited tokens were either
  wrongly blocked or the check was meaningless.
- **After:** The daemon looks up the target message first, finds its real
  room, and checks *that* against your token. Approval-queue actions are
  re-checked the same way at execution time.
- **Files:** `internal/ipc/socket.go`

### Fix 6 — Queue IDs collided, send had no limits
**Commit `9048624`**

- **Before:** Approval-queue IDs were `queue_ + current second` — two sends in
  the same second overwrote each other. Messages of any size/platform were
  accepted.
- **After:** Unique ULID queue IDs. Sends are validated: known platform only,
  room ≤ 200 chars, text 1–4000 chars.
- **Files:** `internal/ipc/socket.go`

### Fix 7 — Network and file-permission hardening
**Commit `c433511`**

- **Before:** The optional HTTP mirror listened on all network interfaces with
  no encryption (tokens in cleartext); the health check hit a locked endpoint
  so Docker always reported "unhealthy"; stale sockets were deleted blindly;
  some config folders were world-readable; Telegram silently used public
  fallback credentials.
- **After:** HTTP binds to localhost by default (with a loud warning if you
  open it up); new public `/healthz` endpoint for Docker; refuses to delete a
  live daemon's socket; folders are `0700`; Telegram warns when using fallback
  credentials.
- **Files:** `internal/ipc/socket.go`, `internal/config/config.go`,
  `internal/db/db.go`, `cmd/daemon.go`, `Dockerfile`

### Fix 8 — Telegram and Matrix could crash the daemon
**Commit `13d7816`**

- **Before:** Telegram assumed every sender was a user (channels/groups
  crashed it). Matrix assumed its sync engine type without checking.
- **After:** Both checked safely; photo-only Telegram messages (no caption)
  are now kept instead of dropped.
- **Files:** `internal/adapters/telegram.go`, `internal/adapters/matrix.go`

### Fix 9 — WhatsApp mixed up chats and broke room filters
**Commit `9a79eda`**

- **Before:** Only the bare number was stored (`12345`), so a personal chat
  `12345@s.whatsapp.net` and a group `12345@g.us` looked identical. Group
  sends failed (wrong server suffix). Watching one room never matched, so
  users saw nothing. Every reconnect added a duplicate listener.
- **After:** Full addresses stored (`whatsapp:12345@g.us`). Room filters
  understand all formats (with/without prefix, old bare numbers still work).
  Replies find the right person. Listener registered once.
- **Files:** `internal/adapters/whatsapp.go`

### Fix 10 — WhatsApp silently threw away most message types
**Commit `7687dd9`**

- **Before:** Only plain text, images, and documents were saved. Voice notes,
  videos, stickers, locations, contacts, polls, and button/list replies were
  **deleted on arrival** — this is the main "WhatsApp doesn't fetch messages"
  bug. Edit markers were lost too.
- **After:** All 12 common types are saved with a readable label
  (`[voice note]`, `[location …]`, `[poll …]`, …). System protocol shells are
  skipped on purpose. Edits are flagged.
- **Files:** `internal/adapters/whatsapp.go`

### Fix 11 — WhatsApp gave no clue about connection or history
**Commit `8ddc6c4`**

- **Before:** Disconnects, logouts, and history syncs were invisible. Every
  media placeholder was the identical string. `ReadHistory` returned a cryptic
  error.
- **After:** Connection events are logged (logout tells you to re-scan the
  QR). Each attachment URI carries its message ID. The history error honestly
  explains: WhatsApp has no server history API — keep the daemon online and
  use `read`/`watch` for the local copy.
- **Files:** `internal/adapters/whatsapp.go`

---

## 4. Why WhatsApp "didn't fetch" — short version

Three stacked causes, all fixed above:

1. **Dropped types** — voice/video/sticker/location/poll replies were
   discarded (Fix 10).
2. **Mixed-up addresses** — groups and DMs shared one ID; group sends and
   room watches failed silently (Fix 9).
3. **No history + no visibility** — offline messages never backfill and nothing
   was logged, so it looked random (Fix 11).

Rule of thumb now: **keep the daemon running** — WhatsApp only delivers live
events; anything arriving while the daemon is off is gone.

---

## 5. How to verify everything

```bash
# Build
go build -o chaind .

# Quick tests
go test ./internal/store/ ./internal/config/ -count=1

# Boot, issue a key, check health
./chaind daemon start
export CHAIND_TOKEN=$(./chaind token issue --role owner --expires 30d)
./chaind status
./chaind doctor
```

All 11 fixes are pushed to `main` on
`https://github.com/fossism/chaind-cli.git` — one commit per fix, in order.
