# chaind Developer Guide

## What is chaind?

`chaind` is a local-first messaging daemon written in Go. It connects to your personal Telegram, Matrix, and WhatsApp accounts as a native client (not a bot), syncs messages into a local SQLite database, and exposes everything through a Unix socket HTTP API. The intended consumers are personal scripts and AI agents that need to read or send messages programmatically without cloud dependencies.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                        chaind daemon                     │
│                                                         │
│  ┌──────────────┐   ┌──────────────┐  ┌─────────────┐  │
│  │  Telegram    │   │   Matrix     │  │  WhatsApp   │  │
│  │  Adapter     │   │   Adapter    │  │  Adapter    │  │
│  └──────┬───────┘   └──────┬───────┘  └──────┬──────┘  │
│         └──────────────────┼─────────────────┘         │
│                     AdapterRouter                       │
│                            │                            │
│              ┌─────────────┴──────────┐                 │
│              │      IPC Server        │                 │
│              │  (HTTP over Unix sock) │                 │
│              └─────────────┬──────────┘                 │
│                            │                            │
│              ┌─────────────┴──────────┐                 │
│              │    SQLite Store (WAL)  │                 │
│              └────────────────────────┘                 │
└─────────────────────────────────────────────────────────┘
         ▲
         │  Unix socket: ~/.config/chaind/chaind.sock
         ▼
   CLI / AI Agent / Script
```

The daemon runs as a background process. CLI commands communicate with it exclusively through the Unix socket using standard HTTP requests.

---

## Project Structure

```
chaind/
├── main.go                    # Entry point — calls cmd.Execute()
├── cmd/                       # Cobra CLI commands
│   ├── root.go                # Root command + config init
│   ├── daemon.go              # `chaind daemon start` — boots everything
│   ├── auth.go                # `chaind auth <platform>` — saves credentials
│   ├── ipc_client.go          # Shared HTTP client dialing the Unix socket
│   ├── ipc.go                 # `chaind ls` — quick message list
│   ├── cli_read.go            # `chaind read` / `chaind watch`
│   ├── cli_send.go            # `chaind send` / `chaind broadcast`
│   ├── cli_search.go          # `chaind search`
│   ├── cli_token.go           # `chaind token issue/list/revoke`
│   ├── cli_queue.go           # `chaind approve list/exec/deny`
│   └── cli_mod.go             # Moderation commands
├── internal/
│   ├── adapters/              # Platform integrations
│   │   ├── interface.go       # Adapter interface definition
│   │   ├── telegram.go        # Telegram via gotd/gotgproto
│   │   ├── matrix.go          # Matrix via mautrix-go
│   │   ├── whatsapp.go        # WhatsApp via whatsmeow
│   │   ├── irc.go             # IRC adapter
│   │   └── mock.go            # Mock adapter for testing
│   ├── auth/
│   │   └── keyring.go         # OS keyring wrapper (zalando/go-keyring)
│   ├── config/
│   │   └── config.go          # Config file read/write (~/.config/chaind/)
│   ├── daemon/
│   │   ├── router.go          # AdapterRouter — dispatches IPC calls to adapters
│   │   └── scheduler.go       # Outbox scheduler — fires scheduled messages
│   ├── db/
│   │   └── db.go              # Low-level DB helpers
│   ├── format/
│   │   ├── parser.go          # Goldmark Markdown parser → AST
│   │   ├── ast.go             # AST node types
│   │   ├── render_telegram.go # Renders AST to Telegram HTML
│   │   ├── render_matrix.go   # Renders AST to Matrix HTML
│   │   └── render_plain.go    # Renders AST to plain text
│   ├── ipc/
│   │   ├── socket.go          # HTTP server over Unix socket + all API handlers
│   │   └── pii.go             # PII scrubbing (email, phone, PAN)
│   ├── models/
│   │   └── task.go            # Task model
│   ├── schema/
│   │   └── message.go         # Canonical Message schema (the unified data model)
│   ├── search/
│   │   └── search.go          # FTS5 full-text search engine
│   └── store/
│       ├── sqlite.go          # Store struct — opens DB, runs migrations
│       ├── schema.go          # SQL DDL (all CREATE TABLE / FTS5 / triggers)
│       ├── writer.go          # StoreWriter — single-threaded write serializer
│       └── repository.go      # Query methods (GetRecentMessages, GetToken, etc.)
```

---

## Core Concepts

### The Adapter Interface

Every platform must implement `internal/adapters/interface.go`:

```go
type Adapter interface {
    Platform() string
    Start(ctx context.Context) error
    Disconnect() error

    ReadHistory(roomID string, limit int, since time.Time) ([]schema.Message, error)
    Watch(ctx context.Context, roomID string) (<-chan schema.Message, error)

    Send(roomID, text string) (schema.Message, error)
    Reply(msgID, text string) (schema.Message, error)
    React(msgID, emoji string) error

    Ban(roomID, userID, reason string) error
    Mute(roomID, userID string, d time.Duration) error
    DeleteMessage(msgID string) error
}
```

When `Start()` is called, the adapter connects to the platform and begins pushing incoming messages into the store via `store.PushMessage()`. The `AdapterRouter` in `internal/daemon/router.go` holds a live registry of all running adapters and routes IPC send/moderate calls to the right one.

### The Canonical Message Schema

All platforms normalize their messages into `internal/schema/message.go`. This is the single JSON shape that the IPC API always returns, regardless of source platform. Key fields:

- `id` — ULID, globally unique across all platforms
- `platform` — `"telegram"`, `"matrix"`, or `"whatsapp"`
- `room` — normalized room with `id`, `name`, `type` (`group|direct|channel`)
- `author` — normalized user with `display_name`, `username`, `is_bot`
- `content` — `type` (`text|image|file|audio|sticker|event`), `text`, optional `html`, `attachments`
- `root_id` / `parent_id` — thread linking via ULIDs

### The Store

`internal/store/sqlite.go` opens two SQLite connections to `~/.local/share/chaind/messages.db`:

- A **read pool** (up to 10 connections) for concurrent queries
- A **single write connection** to avoid WAL contention

All writes go through `StoreWriter` (`store/writer.go`), a channel-based serializer that ensures only one goroutine ever writes to SQLite. Adapters call `store.PushMessage(msg)` which enqueues to this channel non-blockingly.

The schema (`store/schema.go`) includes:
- `messages`, `rooms`, `users` — core data
- `messages_fts` — FTS5 virtual table with auto-sync triggers for full-text search
- `tokens` — capability tokens with tier, room scoping, PII scrub config
- `approval_queue` — human-in-the-loop pending actions
- `outbox` — scheduled messages
- `sync_cursors` — per-platform/room backfill cursors
- `modlog`, `access_log`, `optout` — audit and privacy tables

### The IPC Server

`internal/ipc/socket.go` runs an HTTP/1.1 server over a Unix socket at `~/.config/chaind/chaind.sock`. All endpoints require a Bearer capability token.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/messages/recent` | Last 50 messages across all platforms |
| GET | `/api/v1/messages/search?q=...` | FTS5 full-text search |
| GET | `/api/v1/messages/watch?platform=...&room=...` | SSE live stream |
| POST | `/api/v1/messages/send` | Send a message |
| POST | `/api/v1/moderate` | Ban a user |
| GET | `/api/v1/adapters/status` | Daemon health check |
| GET | `/api/v1/queue` | List pending approval queue items |
| POST | `/api/v1/queue/exec?id=...` | Execute a queued action |
| POST | `/api/v1/queue/deny?id=...` | Deny a queued action |

The `requireToken` middleware validates the token against the `tokens` table, enforces room-level scoping, and optionally applies regex-based PII redaction on read responses.

### Capability Tokens

Tokens are stored in the `tokens` table and have:
- `tier` — `0` = owner (wildcard access), higher tiers are scoped
- `rooms` — comma-separated room IDs, or `"*"` for all
- `pii_scrub` — comma-separated PII categories to redact (`email`, `phone`, `pan`)
- `revoked` — boolean kill switch

Issue a token:
```bash
export CHAIND_TOKEN=$(./chaind token issue --role owner)
```

### Adapter Supervision

Each adapter runs inside `supervise()` in `cmd/daemon.go`. If an adapter's `Start()` returns an error, it's unregistered from the router and retried with exponential backoff (5s → 5min). This keeps the daemon alive even if one platform goes down.

### Human-in-the-Loop (HitL) Queue

Any send request with `"require_approval": true` is written to the `approval_queue` table instead of being dispatched immediately. A human operator reviews it via:

```bash
chaind approve list
chaind approve exec <id>   # or deny <id>
```

### Scheduled Messages (Outbox)

Messages can be scheduled by inserting into the `outbox` table with a `scheduled_at` timestamp. The scheduler in `internal/daemon/scheduler.go` polls every 30 seconds and dispatches any due messages through the router.

---

## Getting Started

### Prerequisites

- Go 1.25+
- OS keyring support (libsecret on Linux, Keychain on macOS, Credential Manager on Windows)

### Build

```bash
git clone https://github.com/fossism/chaind-cli.git
cd chaind-cli
go build -o chaind .
```

### First Run

```bash
# 1. Start the daemon
./chaind daemon start

# 2. Authenticate platforms
./chaind auth telegram   # prompts for MTProto token
./chaind auth matrix     # prompts for access token

# 3. Issue a capability token
export CHAIND_TOKEN=$(./chaind token issue --role owner)

# 4. Read recent messages
./chaind read

# 5. Watch live messages
./chaind watch --platform telegram

# 6. Send a message
./chaind send --platform matrix --room "!room:example.com" --text "Hello"
```

### Configuration

Config file lives at `~/.config/chaind/config.toml`. Environment variables override config:

| Variable | Purpose |
|----------|---------|
| `CHAIND_TOKEN` | Default capability token for CLI commands |
| `CHAIND_MATRIX_HOMESERVER` | Matrix homeserver URL (default: `https://matrix.org`) |
| `CHAIND_MATRIX_USER_ID` | Matrix user ID |
| `CHAIND_TELEGRAM_API_ID` | Telegram API ID |
| `CHAIND_TELEGRAM_API_HASH` | Telegram API hash |
| `CHAIND_WHATSAPP_ENABLED` | Set `true` to enable WhatsApp |
| `CHAIND_WHATSAPP_ACCEPTED_RISK` | Set `true` to acknowledge ToS risk |
| `CHAIND_PREFER_HTTP` | Set `true` to also expose HTTP on `CHAIND_HTTP_PORT` (default `7432`) |

### Data Locations

| Path | Contents |
|------|---------|
| `~/.local/share/chaind/messages.db` | SQLite database |
| `~/.config/chaind/chaind.sock` | Unix socket |
| `~/.config/chaind/config.toml` | Config file |

---

## Adding a New Platform Adapter

1. Create `internal/adapters/myplatform.go`
2. Implement all methods of the `Adapter` interface
3. In `Start()`, connect to the platform and push incoming messages via `store.PushMessage()`
4. In `cmd/daemon.go`, add initialization logic and call `go supervise(gCtx, "myplatform", adapter, router)`

The mock adapter (`internal/adapters/mock.go`) is a good reference for the minimal implementation.

---

## Running Tests

```bash
go test ./...
```

Key test files:
- `internal/config/config_test.go`
- `internal/db/db_test.go`
- `internal/store/repository_test.go`, `writer_test.go`
- `internal/schema/message_test.go`, `schema_test.go`
- `internal/format/format_test.go`

---

## Docker

A `Dockerfile` and `docker-compose.yml` are included. Set `CHAIND_PREFER_HTTP=true` to expose the API over HTTP instead of a Unix socket when running in a container.

```bash
docker-compose up
```
