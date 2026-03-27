# chaind

`chaind` is a sovereign data daemon that interfaces with your personal chat accounts (Telegram, Matrix, WhatsApp) and exposes them as a unified, local-first API over a secure Unix socket. 

It is designed for personal scripts and AI agents to programmatically query messaging history and stream live events without relying on cloud webhooks, third-party databases, or second-class bot frameworks.

## Features

- **Local-first**: Data is synced directly to a local, high-concurrency [SQLite (WAL)](https://sqlite.org/wal.html) database.
- **Native clients**: Logs in directly through Telegram (MTProto/`gotd`), Matrix (`mautrix-go`), and WhatsApp (Multi-device/`whatsmeow`) as an actively linked device.
- **Unified IPC**: Exposes a single, normalized JSON schema via a Unix domain socket (`/tmp/chaind.sock`), abstracting away platform-specific protocol details.
- **Zero dependencies**: Distributed as a single, statically linked binary compiled in pure Go (no CGO overhead).

## Installation

Download the binary from the [releases page](), or compile it from source (requires Go 1.25+):

```bash
git clone https://github.com/fossism/chaind-cli.git
cd chaind-cli
go build -o chaind .
```

## Quick start

1. Boot the background daemon to initialize the storage and socket bindings:
   ```bash
   ./chaind daemon start
   ```

2. Authenticate the active network bridges:
   ```bash
   ./chaind auth telegram
   ./chaind auth matrix
   ```

3. Generate a local capability token for IPC access:
   ```bash
   export CHAIND_TOKEN=$(./chaind token issue --role owner)
   ```

4. Monitor the live Server-Sent Events (SSE) stream of incoming messages:
   ```bash
   ./chaind watch --platform telegram 
   ```

5. Dispatch a message to an active platform via the IPC router:
   ```bash
   ./chaind send --platform matrix --room "!room_id:example.com" --text "Hello world."
   ```

## License
GPL-3.0
