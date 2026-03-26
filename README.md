# chaind

A CLI built in Go to sync, track, and manage GitHub issues directly from the terminal. Built for developers who prefer to stay out of the browser.

## Why?

Developers spend most of their time in the terminal and text editors. However, issue tracking, project management, and checking assigned tasks happen in clunky web dashboards. Constant context-switching between the terminal and browser breaks focus.

`chaind` solves this by syncing your assigned issues and pull requests into a local-first interface, bridging the web out of your local dev experience.

## Features

- **Offline-First Storage:** Syncs tasks from GitHub directly to a local high-performance SQLite database.
- **Workflow Commands:** Simple, chained commands to check assigned issues (`ls`), start working (`start`), and mark them closed (`done`).
- **Dependency-Free:** Fast, compiled binary that works identically across macOS, Linux, and Windows.

## Installation

Using the Go toolchain:

```bash
go install github.com/fossism/chaind-cli@latest
```

Or build from source:

```bash
git clone https://github.com/fossism/chaind-cli.git
cd chaind-cli
go build -o chaind

# Optional: move to your PATH
mv chaind /usr/local/bin/
```

## Usage

**1. Authentication:** Set up and securely cache your GitHub Personal Access Token (PAT).
```bash
chaind auth
```

**2. Sync:** Pull down assigned issues and PRs into the local database.
```bash
chaind sync
```

**3. List:** View your tasks in the terminal.
```bash
chaind ls
```

**4. Start Session:** Mark an issue as works-in-progress.
```bash
chaind start <repo> <issue_number>
```

**5. Resolve:** Mark an issue as completed.
```bash
chaind done <repo> <issue_number>
```

## Technical Architecture

- **Go:** Core application (v1.20+)
- **Cobra:** CLI framework for command routing
- **go-pretty:** Terminal formatting and tables
- **SQLite + GORM:** Embedded relational tables for local caching

## Roadmap: Federated Matrix Bridge

A primary issue in open-source tracking is scattered fragmentation across chat apps. Users report bugs via Matrix, Telegram, or WhatsApp, and maintainers lose track. 

`chaind` is planned to act as a federated client overlay. By bridging Matrix (and `mautrix-whatsapp` / `mautrix-telegram`), `@mentions` and issues from chat apps will appear seamlessly in the terminal's task list, allowing bidirectional sync when marked as `done`.
